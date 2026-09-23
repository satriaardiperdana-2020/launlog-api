package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository/postgresql"
)

var (
	ErrInvalidPayment             = errors.New("invalid payment request")
	ErrPaymentNotFound            = errors.New("order not found")
	ErrPaymentOverpayment         = errors.New("payment exceeds the outstanding balance")
	ErrPaymentNoBalance           = errors.New("order has no outstanding balance")
	ErrPaymentOrderCancelled      = errors.New("cancelled orders cannot receive payments")
	ErrPaymentIdempotencyConflict = errors.New("payment idempotency key was reused with a different request")
	ErrPaymentNotVoidable         = errors.New("only confirmed payments can be voided")
	ErrInvalidPaymentVoid         = errors.New("payment void reason is required")
	ErrPaymentLedgerInvariant     = errors.New("confirmed payments exceed the order total")
	ErrPaymentRefundsUnsupported  = errors.New("refund records exist but refund policy is not approved")
)

type RecordPaymentInput struct {
	TenderedAmount    int64
	Method            string
	ExternalReference *string
	Notes             *string
	IdempotencyKey    string
	RequestHash       []byte
}

type Payment struct {
	ID                int64      `json:"id"`
	Amount            int64      `json:"amount"`
	Method            string     `json:"method"`
	Status            string     `json:"status"`
	ExternalReference *string    `json:"external_reference"`
	Notes             *string    `json:"notes"`
	ConfirmedAt       *time.Time `json:"confirmed_at"`
	VoidedAt          *time.Time `json:"voided_at"`
	VoidedBy          *int64     `json:"voided_by"`
	VoidReason        *string    `json:"void_reason"`
	CreatedBy         int64      `json:"created_by"`
	CreatedAt         time.Time  `json:"created_at"`
}

type PaymentRecordResult struct {
	Payment           Payment `json:"payment"`
	PaymentStatus     string  `json:"payment_status"`
	PaidAmount        int64   `json:"paid_amount"`
	OutstandingAmount int64   `json:"outstanding_amount"`
	TenderedAmount    int64   `json:"tendered_amount"`
	ChangeAmount      int64   `json:"change_amount"`
	Replayed          bool    `json:"-"`
}

type PaymentListResult struct {
	OrderID           int64     `json:"order_id"`
	OrderTotal        int64     `json:"order_total"`
	PaymentStatus     string    `json:"payment_status"`
	PaidAmount        int64     `json:"paid_amount"`
	OutstandingAmount int64     `json:"outstanding_amount"`
	Items             []Payment `json:"items"`
	TotalItems        int64     `json:"-"`
}

type PaymentVoidResult struct {
	Payment           Payment `json:"payment"`
	PaymentStatus     string  `json:"payment_status"`
	PaidAmount        int64   `json:"paid_amount"`
	OutstandingAmount int64   `json:"outstanding_amount"`
}

type PaymentCatalog struct{ database *repository.Postgres }

func NewPaymentCatalog(database *repository.Postgres) *PaymentCatalog {
	return &PaymentCatalog{database: database}
}

func validPaymentMethod(method string) bool {
	switch method {
	case "CASH", "BCA_TRANSFER", "QRIS":
		return true
	default:
		return false
	}
}

func paymentAmounts(method string, tendered, outstanding int64) (applied, change int64, err error) {
	if !validPaymentMethod(method) || tendered <= 0 {
		return 0, 0, ErrInvalidPayment
	}
	if outstanding <= 0 {
		return 0, 0, ErrPaymentNoBalance
	}
	if method == "CASH" {
		applied = tendered
		if applied > outstanding {
			applied = outstanding
		}
		return applied, tendered - applied, nil
	}
	if tendered > outstanding {
		return 0, 0, ErrPaymentOverpayment
	}
	return tendered, 0, nil
}

func paymentStatusFor(total, paid int64) string {
	if paid == 0 {
		return "UNPAID"
	}
	if paid == total {
		return "PAID"
	}
	return "PARTIALLY_PAID"
}

func (s *PaymentCatalog) Record(ctx context.Context, actor Actor, orderID int64, input RecordPaymentInput) (PaymentRecordResult, error) {
	if orderID < 1 || actor.BusinessID < 1 || actor.UserID < 1 || input.TenderedAmount <= 0 ||
		!validPaymentMethod(input.Method) || len(input.IdempotencyKey) < 1 || len(input.IdempotencyKey) > 128 || len(input.RequestHash) != 32 {
		return PaymentRecordResult{}, ErrInvalidPayment
	}
	if exceedsPaymentTextLimit(input.ExternalReference, 256) || exceedsPaymentTextLimit(input.Notes, 2000) {
		return PaymentRecordResult{}, ErrInvalidPayment
	}
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return PaymentRecordResult{}, err
	}
	defer tx.Rollback(ctx)
	q := s.database.Queries().WithTx(tx)
	order, err := lockLifecycleOrder(ctx, q, actor, orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentRecordResult{}, ErrPaymentNotFound
	}
	if errors.Is(err, ErrOrderOutletForbidden) {
		return PaymentRecordResult{}, err
	}
	if err != nil {
		return PaymentRecordResult{}, err
	}
	if order.Status == "CANCELLED" || order.DeletedAt.Valid {
		return PaymentRecordResult{}, ErrPaymentOrderCancelled
	}
	if err := ensureNoPaymentRefunds(ctx, q, actor.BusinessID, order.OutletID, orderID); err != nil {
		return PaymentRecordResult{}, err
	}

	// The order row is the serialization point for payment attempts, idempotent
	// retries, and order cancellation. A unique index remains the final backstop.
	existing, err := q.GetPaymentByIdempotencyKey(ctx, postgresql.GetPaymentByIdempotencyKeyParams{
		BusinessID: actor.BusinessID, OutletID: order.OutletID, OrderID: orderID,
		IdempotencyKey: pgtype.Text{String: input.IdempotencyKey, Valid: true},
	})
	if err == nil {
		if !bytes.Equal(existing.RequestHash, input.RequestHash) {
			return PaymentRecordResult{}, ErrPaymentIdempotencyConflict
		}
		paid, err := q.GetConfirmedPaymentTotal(ctx, postgresql.GetConfirmedPaymentTotalParams{BusinessID: actor.BusinessID, OutletID: order.OutletID, OrderID: orderID})
		if err != nil {
			return PaymentRecordResult{}, err
		}
		if paid > order.TotalAmount {
			return PaymentRecordResult{}, ErrPaymentLedgerInvariant
		}
		if err := tx.Commit(ctx); err != nil {
			return PaymentRecordResult{}, err
		}
		return PaymentRecordResult{
			Payment:       paymentFrom(existing.ID, existing.Amount, existing.Method, existing.Status, existing.ExternalReference, existing.Notes, existing.ConfirmedAt, existing.VoidedAt, existing.VoidedBy, existing.VoidReason, existing.CreatedBy, existing.CreatedAt),
			PaymentStatus: paymentStatusFor(order.TotalAmount, paid), PaidAmount: paid, OutstandingAmount: order.TotalAmount - paid,
			TenderedAmount: input.TenderedAmount, ChangeAmount: changeFor(existing.Method, input.TenderedAmount, existing.Amount), Replayed: true,
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return PaymentRecordResult{}, err
	}

	paidBefore, err := q.GetConfirmedPaymentTotal(ctx, postgresql.GetConfirmedPaymentTotalParams{BusinessID: actor.BusinessID, OutletID: order.OutletID, OrderID: orderID})
	if err != nil {
		return PaymentRecordResult{}, err
	}
	if paidBefore > order.TotalAmount {
		return PaymentRecordResult{}, ErrPaymentLedgerInvariant
	}
	outstanding := order.TotalAmount - paidBefore
	if outstanding <= 0 {
		return PaymentRecordResult{}, ErrPaymentNoBalance
	}
	amountApplied, changeAmount, err := paymentAmounts(input.Method, input.TenderedAmount, outstanding)
	if err != nil {
		return PaymentRecordResult{}, err
	}
	created, err := q.CreateConfirmedPayment(ctx, postgresql.CreateConfirmedPaymentParams{
		BusinessID: actor.BusinessID, OutletID: order.OutletID, OrderID: orderID, Amount: amountApplied,
		Method: input.Method, ExternalReference: optionalText(input.ExternalReference), Notes: optionalText(input.Notes),
		CreatedBy: actor.UserID, IdempotencyKey: pgtype.Text{String: input.IdempotencyKey, Valid: true}, RequestHash: input.RequestHash,
	})
	if err != nil {
		return PaymentRecordResult{}, err
	}
	paidAfter := paidBefore + amountApplied // bounded by order.TotalAmount, so no int64 overflow
	status := paymentStatusFor(order.TotalAmount, paidAfter)
	updated, err := q.UpdateOrderPaymentStatus(ctx, postgresql.UpdateOrderPaymentStatusParams{
		BusinessID: actor.BusinessID, OutletID: order.OutletID, OrderID: orderID, PaymentStatus: status,
	})
	if err != nil {
		return PaymentRecordResult{}, err
	}
	oldValues, err := json.Marshal(map[string]any{"payment_status": paymentStatusFor(order.TotalAmount, paidBefore), "paid_amount": paidBefore, "outstanding_amount": outstanding})
	if err != nil {
		return PaymentRecordResult{}, err
	}
	newValues, err := json.Marshal(map[string]any{"payment_id": created.ID, "method": input.Method, "amount": amountApplied,
		"payment_status": status, "paid_amount": paidAfter, "outstanding_amount": order.TotalAmount - paidAfter})
	if err != nil {
		return PaymentRecordResult{}, err
	}
	if err := insertPaymentAudit(ctx, q, actor, order.OutletID, created.ID, "PAYMENT_CONFIRMED", oldValues, newValues); err != nil {
		return PaymentRecordResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PaymentRecordResult{}, err
	}
	return PaymentRecordResult{
		Payment:       paymentFrom(created.ID, created.Amount, created.Method, created.Status, created.ExternalReference, created.Notes, created.ConfirmedAt, pgtype.Timestamptz{}, pgtype.Int8{}, pgtype.Text{}, created.CreatedBy, created.CreatedAt),
		PaymentStatus: updated.PaymentStatus, PaidAmount: paidAfter, OutstandingAmount: order.TotalAmount - paidAfter,
		TenderedAmount: input.TenderedAmount, ChangeAmount: changeAmount,
	}, nil
}

func changeFor(method string, tendered, applied int64) int64 {
	if method == "CASH" {
		return tendered - applied
	}
	return 0
}

func exceedsPaymentTextLimit(value *string, maxRunes int) bool {
	return value != nil && utf8.RuneCountInString(*value) > maxRunes
}

func paymentFrom(id, amount int64, method, status string, externalReference, notes pgtype.Text,
	confirmedAt, voidedAt pgtype.Timestamptz, voidedBy pgtype.Int8, voidReason pgtype.Text, createdBy int64, createdAt pgtype.Timestamptz) Payment {
	var confirmed *time.Time
	if confirmedAt.Valid {
		value := confirmedAt.Time
		confirmed = &value
	}
	var voided *time.Time
	if voidedAt.Valid {
		value := voidedAt.Time
		voided = &value
	}
	var voidActor *int64
	if voidedBy.Valid {
		value := voidedBy.Int64
		voidActor = &value
	}
	return Payment{ID: id, Amount: amount, Method: method, Status: status,
		ExternalReference: textPointer(externalReference), Notes: textPointer(notes), ConfirmedAt: confirmed,
		VoidedAt: voided, VoidedBy: voidActor, VoidReason: textPointer(voidReason),
		CreatedBy: createdBy, CreatedAt: createdAt.Time}
}

func (s *PaymentCatalog) List(ctx context.Context, actor Actor, orderID int64, pageOffset, pageLimit int32) (PaymentListResult, error) {
	if orderID < 1 || actor.BusinessID < 1 || actor.UserID < 1 {
		return PaymentListResult{}, ErrPaymentNotFound
	}
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return PaymentListResult{}, err
	}
	defer tx.Rollback(ctx)
	q := s.database.Queries().WithTx(tx)
	order, err := lockLifecycleOrder(ctx, q, actor, orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentListResult{}, ErrPaymentNotFound
	}
	if errors.Is(err, ErrOrderOutletForbidden) {
		return PaymentListResult{}, err
	}
	if err != nil {
		return PaymentListResult{}, err
	}
	if err := ensureNoPaymentRefunds(ctx, q, actor.BusinessID, order.OutletID, orderID); err != nil {
		return PaymentListResult{}, err
	}
	paid, err := q.GetConfirmedPaymentTotal(ctx, postgresql.GetConfirmedPaymentTotalParams{BusinessID: actor.BusinessID, OutletID: order.OutletID, OrderID: orderID})
	if err != nil {
		return PaymentListResult{}, err
	}
	if paid > order.TotalAmount {
		return PaymentListResult{}, ErrPaymentLedgerInvariant
	}
	rows, err := q.ListOrderPayments(ctx, postgresql.ListOrderPaymentsParams{BusinessID: actor.BusinessID, OutletID: order.OutletID, OrderID: orderID, PageLimit: pageLimit, PageOffset: pageOffset})
	if err != nil {
		return PaymentListResult{}, err
	}
	totalItems, err := q.CountOrderPayments(ctx, postgresql.CountOrderPaymentsParams{BusinessID: actor.BusinessID, OutletID: order.OutletID, OrderID: orderID})
	if err != nil {
		return PaymentListResult{}, err
	}
	items := make([]Payment, 0, len(rows))
	for _, row := range rows {
		items = append(items, paymentFrom(row.ID, row.Amount, row.Method, row.Status, row.ExternalReference, row.Notes, row.ConfirmedAt, row.VoidedAt, row.VoidedBy, row.VoidReason, row.CreatedBy, row.CreatedAt))
	}
	return PaymentListResult{OrderID: orderID, OrderTotal: order.TotalAmount, PaymentStatus: paymentStatusFor(order.TotalAmount, paid),
		PaidAmount: paid, OutstandingAmount: order.TotalAmount - paid, Items: items, TotalItems: totalItems}, nil
}

// Void reverses an erroneous receipt entry; it does not create or execute a
// refund. Both it and order cancellation serialize on the order row.
func (s *PaymentCatalog) Void(ctx context.Context, actor Actor, orderID, paymentID int64, reason string) (PaymentVoidResult, error) {
	reason = strings.TrimSpace(reason)
	if orderID < 1 || paymentID < 1 || actor.BusinessID < 1 || actor.UserID < 1 || strings.TrimSpace(reason) == "" || utf8.RuneCountInString(reason) > 2000 {
		return PaymentVoidResult{}, ErrInvalidPaymentVoid
	}
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return PaymentVoidResult{}, err
	}
	defer tx.Rollback(ctx)
	q := s.database.Queries().WithTx(tx)
	order, err := lockLifecycleOrder(ctx, q, actor, orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentVoidResult{}, ErrPaymentNotFound
	}
	if errors.Is(err, ErrOrderOutletForbidden) {
		return PaymentVoidResult{}, err
	}
	if err != nil {
		return PaymentVoidResult{}, err
	}
	if err := ensureNoPaymentRefunds(ctx, q, actor.BusinessID, order.OutletID, orderID); err != nil {
		return PaymentVoidResult{}, err
	}
	payment, err := q.GetOrderPaymentForUpdate(ctx, postgresql.GetOrderPaymentForUpdateParams{
		BusinessID: actor.BusinessID, OutletID: order.OutletID, OrderID: orderID, PaymentID: paymentID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentVoidResult{}, ErrPaymentNotFound
	}
	if err != nil {
		return PaymentVoidResult{}, err
	}
	if payment.Status == "VOIDED" && payment.VoidedBy.Valid && payment.VoidedBy.Int64 == actor.UserID && payment.VoidReason.Valid && payment.VoidReason.String == reason {
		paid, err := q.GetConfirmedPaymentTotal(ctx, postgresql.GetConfirmedPaymentTotalParams{BusinessID: actor.BusinessID, OutletID: order.OutletID, OrderID: orderID})
		if err != nil {
			return PaymentVoidResult{}, err
		}
		if paid > order.TotalAmount {
			return PaymentVoidResult{}, ErrPaymentLedgerInvariant
		}
		if err := tx.Commit(ctx); err != nil {
			return PaymentVoidResult{}, err
		}
		return PaymentVoidResult{Payment: paymentFrom(payment.ID, payment.Amount, payment.Method, payment.Status, payment.ExternalReference, payment.Notes,
			payment.ConfirmedAt, payment.VoidedAt, payment.VoidedBy, payment.VoidReason, payment.CreatedBy, payment.CreatedAt),
			PaymentStatus: paymentStatusFor(order.TotalAmount, paid), PaidAmount: paid, OutstandingAmount: order.TotalAmount - paid}, nil
	}
	if payment.Status != "CONFIRMED" {
		return PaymentVoidResult{}, ErrPaymentNotVoidable
	}
	voided, err := q.VoidConfirmedPayment(ctx, postgresql.VoidConfirmedPaymentParams{
		BusinessID: actor.BusinessID, OutletID: order.OutletID, OrderID: orderID, PaymentID: paymentID,
		VoidedBy: pgtype.Int8{Int64: actor.UserID, Valid: true}, VoidReason: pgtype.Text{String: reason, Valid: true},
	})
	if err != nil {
		return PaymentVoidResult{}, err
	}
	paid, err := q.GetConfirmedPaymentTotal(ctx, postgresql.GetConfirmedPaymentTotalParams{BusinessID: actor.BusinessID, OutletID: order.OutletID, OrderID: orderID})
	if err != nil {
		return PaymentVoidResult{}, err
	}
	if paid > order.TotalAmount {
		return PaymentVoidResult{}, ErrPaymentLedgerInvariant
	}
	status := paymentStatusFor(order.TotalAmount, paid)
	updated, err := q.UpdateOrderPaymentStatus(ctx, postgresql.UpdateOrderPaymentStatusParams{
		BusinessID: actor.BusinessID, OutletID: order.OutletID, OrderID: orderID, PaymentStatus: status,
	})
	if err != nil {
		return PaymentVoidResult{}, err
	}
	oldValues, err := json.Marshal(map[string]any{"payment_id": paymentID, "amount": payment.Amount, "status": payment.Status, "payment_status": order.PaymentStatus})
	if err != nil {
		return PaymentVoidResult{}, err
	}
	newValues, err := json.Marshal(map[string]any{"payment_id": paymentID, "amount": payment.Amount, "status": voided.Status,
		"voided_by": actor.UserID, "void_reason": reason, "payment_status": updated.PaymentStatus, "paid_amount": paid,
		"outstanding_amount": order.TotalAmount - paid})
	if err != nil {
		return PaymentVoidResult{}, err
	}
	if err := insertPaymentAudit(ctx, q, actor, order.OutletID, paymentID, "PAYMENT_VOIDED", oldValues, newValues); err != nil {
		return PaymentVoidResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PaymentVoidResult{}, err
	}
	return PaymentVoidResult{Payment: paymentFrom(voided.ID, voided.Amount, voided.Method, voided.Status, voided.ExternalReference,
		voided.Notes, voided.ConfirmedAt, voided.VoidedAt, voided.VoidedBy, voided.VoidReason, voided.CreatedBy, voided.CreatedAt),
		PaymentStatus: updated.PaymentStatus, PaidAmount: paid, OutstandingAmount: order.TotalAmount - paid}, nil
}

func ensureNoPaymentRefunds(ctx context.Context, q *postgresql.Queries, businessID, outletID, orderID int64) error {
	count, err := q.CountOrderPaymentRefunds(ctx, postgresql.CountOrderPaymentRefundsParams{BusinessID: businessID, OutletID: outletID, OrderID: orderID})
	if err != nil {
		return err
	}
	if count != 0 {
		return ErrPaymentRefundsUnsupported
	}
	return nil
}

func insertPaymentAudit(ctx context.Context, q *postgresql.Queries, actor Actor, outletID, paymentID int64, action string, oldValues, newValues []byte) error {
	userAgent := actor.UserAgent
	if len(userAgent) > 512 {
		userAgent = userAgent[:512]
	}
	return q.InsertPaymentAuditLog(ctx, postgresql.InsertPaymentAuditLogParams{BusinessID: actor.BusinessID,
		OutletID: pgtype.Int8{Int64: outletID, Valid: true}, ActorUserID: pgtype.Int8{Int64: actor.UserID, Valid: true},
		Action: action, PaymentID: pgtype.Int8{Int64: paymentID, Valid: true}, OldValues: oldValues,
		NewValues: newValues, IpAddress: actor.IP, UserAgent: pgtype.Text{String: userAgent, Valid: userAgent != ""}})
}

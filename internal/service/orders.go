package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository/postgresql"
)

var (
	ErrInvalidOrder             = errors.New("invalid order request")
	ErrOrderNotFound            = errors.New("order not found")
	ErrOrderOutletForbidden     = errors.New("outlet is not assigned to this user")
	ErrOrderCustomerNotFound    = errors.New("active customer not found")
	ErrOrderServiceNotFound     = errors.New("active service not found")
	ErrOrderPerfumeNotFound     = errors.New("active perfume not found")
	ErrOrderIdempotencyConflict = errors.New("idempotency key was already used with a different payload")
	ErrOrderTotalOverflow       = errors.New("order total exceeds the supported rupiah range")
	ErrInvalidOrderTransition   = errors.New("order status transition is not allowed")
	ErrOrderRequiresRefund      = errors.New("orders with payments cannot be cancelled until a refund policy is approved")
	ErrInvalidCancellation      = errors.New("cancellation reason is required")
)

type OrderItemInput struct {
	ServiceID int64
	Quantity  string
	Notes     *string
}

type CreateOrderInput struct {
	OutletID       int64
	CustomerID     int64
	PerfumeID      *int64
	DueAt          *time.Time
	Notes          *string
	Items          []OrderItemInput
	IdempotencyKey string
	RequestHash    []byte
}

type OrderItem struct {
	ID                      int64   `json:"id"`
	ServiceID               int64   `json:"service_id"`
	PerfumeID               *int64  `json:"perfume_id"`
	ServiceNameSnapshot     string  `json:"service_name_snapshot"`
	ServiceUnitSnapshot     string  `json:"service_unit_snapshot"`
	UnitPriceAmountSnapshot int64   `json:"unit_price_amount_snapshot"`
	PerfumeNameSnapshot     *string `json:"perfume_name_snapshot"`
	Quantity                string  `json:"quantity"`
	LineTotalAmount         int64   `json:"line_total_amount"`
	Notes                   *string `json:"notes"`
}

type Order struct {
	ID            int64       `json:"id"`
	BusinessID    int64       `json:"business_id"`
	OutletID      int64       `json:"outlet_id"`
	CustomerID    int64       `json:"customer_id"`
	InvoiceNumber string      `json:"invoice_number"`
	Status        string      `json:"status"`
	PaymentStatus string      `json:"payment_status"`
	TotalAmount   int64       `json:"total_amount"`
	Notes         *string     `json:"notes"`
	ReceivedAt    time.Time   `json:"received_at"`
	DueAt         *time.Time  `json:"due_at"`
	CreatedBy     int64       `json:"created_by"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
	Items         []OrderItem `json:"items"`
}

type OrderSummary struct {
	ID            int64      `json:"id"`
	BusinessID    int64      `json:"business_id"`
	OutletID      int64      `json:"outlet_id"`
	CustomerID    int64      `json:"customer_id"`
	InvoiceNumber string     `json:"invoice_number"`
	Status        string     `json:"status"`
	PaymentStatus string     `json:"payment_status"`
	TotalAmount   int64      `json:"total_amount"`
	Notes         *string    `json:"notes"`
	ReceivedAt    time.Time  `json:"received_at"`
	DueAt         *time.Time `json:"due_at"`
	CreatedBy     int64      `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type OrderCreationResult struct {
	Order    Order
	Replayed bool
}

type OrderLifecycleResult struct {
	OrderID            int64      `json:"order_id"`
	Status             string     `json:"status"`
	PaymentStatus      string     `json:"payment_status"`
	UpdatedAt          time.Time  `json:"updated_at"`
	CompletedAt        *time.Time `json:"completed_at"`
	CancelledAt        *time.Time `json:"cancelled_at"`
	CancellationReason *string    `json:"cancellation_reason"`
}

type OrderStatusHistoryEntry struct {
	ID            int64     `json:"id"`
	FromStatus    *string   `json:"from_status"`
	ToStatus      string    `json:"to_status"`
	ChangedBy     int64     `json:"changed_by"`
	ChangedByName string    `json:"changed_by_name"`
	Notes         *string   `json:"notes"`
	ChangedAt     time.Time `json:"changed_at"`
}

type OrderFilter struct {
	OutletID   *int64
	Status     *string
	PageOffset int32
	PageLimit  int32
}

type OrderCatalog struct{ database *repository.Postgres }

func NewOrderCatalog(database *repository.Postgres) *OrderCatalog {
	return &OrderCatalog{database: database}
}

func (s *OrderCatalog) Create(ctx context.Context, actor Actor, input CreateOrderInput) (OrderCreationResult, error) {
	if input.OutletID < 1 || input.CustomerID < 1 || len(input.Items) == 0 || len(input.IdempotencyKey) == 0 || len(input.RequestHash) != 32 {
		return OrderCreationResult{}, ErrInvalidOrder
	}
	if actor.BusinessID < 1 || actor.UserID < 1 {
		return OrderCreationResult{}, ErrInvalidOrder
	}
	if actor.Role != "ADMIN" && !containsOutlet(actor.OutletIDs, input.OutletID) {
		return OrderCreationResult{}, ErrOrderOutletForbidden
	}
	for _, item := range input.Items {
		if item.ServiceID < 1 {
			return OrderCreationResult{}, ErrInvalidOrder
		}
	}
	if input.PerfumeID != nil && *input.PerfumeID < 1 {
		return OrderCreationResult{}, ErrInvalidOrder
	}

	tx, err := s.database.Begin(ctx)
	if err != nil {
		return OrderCreationResult{}, err
	}
	defer tx.Rollback(ctx)
	q := s.database.Queries().WithTx(tx)
	lockKey := fmt.Sprintf("%d:%d:%s", actor.BusinessID, input.OutletID, input.IdempotencyKey)
	if err := q.LockOrderIdempotency(ctx, lockKey); err != nil {
		return OrderCreationResult{}, err
	}

	outlet, err := q.GetActiveOrderOutlet(ctx, postgresql.GetActiveOrderOutletParams{BusinessID: actor.BusinessID, OutletID: input.OutletID})
	if errors.Is(err, pgx.ErrNoRows) {
		return OrderCreationResult{}, ErrOrderNotFound
	}
	if err != nil {
		return OrderCreationResult{}, err
	}
	if actor.Role != "ADMIN" {
		if _, err := q.GetActiveStaffOrderOutlet(ctx, postgresql.GetActiveStaffOrderOutletParams{
			BusinessID: actor.BusinessID, UserID: actor.UserID, OutletID: input.OutletID,
		}); errors.Is(err, pgx.ErrNoRows) {
			return OrderCreationResult{}, ErrOrderOutletForbidden
		} else if err != nil {
			return OrderCreationResult{}, err
		}
	}

	existing, err := q.GetOrderByIdempotencyKey(ctx, postgresql.GetOrderByIdempotencyKeyParams{
		BusinessID: actor.BusinessID, OutletID: input.OutletID,
		IdempotencyKey: pgtype.Text{String: input.IdempotencyKey, Valid: true},
	})
	if err == nil {
		if !bytes.Equal(existing.RequestHash, input.RequestHash) {
			return OrderCreationResult{}, ErrOrderIdempotencyConflict
		}
		order, err := loadOrder(ctx, q, existing.ID, existing.BusinessID, existing.OutletID, existing.CustomerID,
			existing.InvoiceNumber, existing.Status, existing.PaymentStatus, existing.TotalAmount, existing.Notes,
			existing.ReceivedAt, existing.DueAt, existing.CreatedBy, existing.CreatedAt, existing.UpdatedAt)
		if err != nil {
			return OrderCreationResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return OrderCreationResult{}, err
		}
		return OrderCreationResult{Order: order, Replayed: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return OrderCreationResult{}, err
	}
	if _, err := q.GetActiveOrderCustomer(ctx, postgresql.GetActiveOrderCustomerParams{BusinessID: actor.BusinessID, CustomerID: input.CustomerID}); errors.Is(err, pgx.ErrNoRows) {
		return OrderCreationResult{}, ErrOrderCustomerNotFound
	} else if err != nil {
		return OrderCreationResult{}, err
	}

	perfumeID := pgtype.Int8{}
	perfumeSnapshot := pgtype.Text{}
	if input.PerfumeID != nil {
		perfume, err := q.GetActiveOrderPerfume(ctx, postgresql.GetActiveOrderPerfumeParams{BusinessID: actor.BusinessID, PerfumeID: *input.PerfumeID})
		if errors.Is(err, pgx.ErrNoRows) {
			return OrderCreationResult{}, ErrOrderPerfumeNotFound
		}
		if err != nil {
			return OrderCreationResult{}, err
		}
		perfumeID = pgtype.Int8{Int64: perfume.ID, Valid: true}
		perfumeSnapshot = pgtype.Text{String: perfume.Name, Valid: true}
	}

	type calculatedLine struct {
		input       OrderItemInput
		serviceName string
		unit        string
		price       int64
		milli       int64
		lineTotal   int64
	}
	lines := make([]calculatedLine, 0, len(input.Items))
	total := new(big.Int)
	for _, requested := range input.Items {
		itemService, err := q.GetActiveOrderService(ctx, postgresql.GetActiveOrderServiceParams{BusinessID: actor.BusinessID, ServiceID: requested.ServiceID})
		if errors.Is(err, pgx.ErrNoRows) {
			return OrderCreationResult{}, ErrOrderServiceNotFound
		}
		if err != nil {
			return OrderCreationResult{}, err
		}
		milli, err := parseMilliQuantity(requested.Quantity)
		if err != nil || (itemService.Unit == "PIECE" && milli%1000 != 0) {
			return OrderCreationResult{}, ErrInvalidOrder
		}
		lineTotal := new(big.Int).Mul(big.NewInt(milli), big.NewInt(itemService.UnitPriceAmount))
		lineTotal.Add(lineTotal, big.NewInt(500)) // PostgreSQL round(numeric) rounds positive half values away from zero.
		lineTotal.Quo(lineTotal, big.NewInt(1000))
		if !lineTotal.IsInt64() {
			return OrderCreationResult{}, ErrOrderTotalOverflow
		}
		total.Add(total, lineTotal)
		if !total.IsInt64() {
			return OrderCreationResult{}, ErrOrderTotalOverflow
		}
		lines = append(lines, calculatedLine{input: requested, serviceName: itemService.Name, unit: itemService.Unit,
			price: itemService.UnitPriceAmount, milli: milli, lineTotal: lineTotal.Int64()})
	}

	orderTime, err := q.GetOrderTime(ctx)
	if err != nil {
		return OrderCreationResult{}, err
	}
	if input.DueAt != nil && input.DueAt.Before(orderTime.ReceivedAt.Time) {
		return OrderCreationResult{}, ErrInvalidOrder
	}
	allocated, err := q.AllocateInvoiceNumber(ctx, postgresql.AllocateInvoiceNumberParams{
		BusinessID: actor.BusinessID, OutletID: input.OutletID, CounterDate: orderTime.CounterDate,
	})
	if err != nil {
		return OrderCreationResult{}, err
	}
	invoiceNumber := fmt.Sprintf("%s-%s-%06d", outlet.Code, allocated.CounterDate.Time.Format("20060102"), allocated.InvoiceSequence)
	dueAt := pgtype.Timestamptz{}
	if input.DueAt != nil {
		dueAt = pgtype.Timestamptz{Time: *input.DueAt, Valid: true}
	}
	orderRow, err := q.CreateOrder(ctx, postgresql.CreateOrderParams{
		BusinessID: actor.BusinessID, OutletID: input.OutletID, CustomerID: input.CustomerID,
		InvoiceNumber: invoiceNumber, TotalAmount: total.Int64(), Notes: optionalText(input.Notes),
		ReceivedAt: orderTime.ReceivedAt, DueAt: dueAt, CreatedBy: actor.UserID,
		IdempotencyKey: pgtype.Text{String: input.IdempotencyKey, Valid: true}, RequestHash: input.RequestHash,
	})
	if err != nil {
		return OrderCreationResult{}, err
	}
	for _, line := range lines {
		err := q.CreateOrderItem(ctx, postgresql.CreateOrderItemParams{
			BusinessID: actor.BusinessID, OutletID: input.OutletID, OrderID: orderRow.ID,
			ServiceID: line.input.ServiceID, PerfumeID: perfumeID,
			ServiceNameSnapshot: line.serviceName, ServiceUnitSnapshot: line.unit,
			UnitPriceAmountSnapshot: line.price, PerfumeNameSnapshot: perfumeSnapshot,
			Quantity:        pgtype.Numeric{Int: big.NewInt(line.milli), Exp: -3, Valid: true},
			LineTotalAmount: line.lineTotal, Notes: optionalText(line.input.Notes),
		})
		if err != nil {
			return OrderCreationResult{}, err
		}
	}
	if err := q.CreateInitialOrderStatus(ctx, postgresql.CreateInitialOrderStatusParams{
		BusinessID: actor.BusinessID, OutletID: input.OutletID, OrderID: orderRow.ID, ChangedBy: actor.UserID,
	}); err != nil {
		return OrderCreationResult{}, err
	}
	auditItems := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		auditItems = append(auditItems, map[string]any{
			"service_id": line.input.ServiceID, "service_name_snapshot": line.serviceName,
			"service_unit_snapshot": line.unit, "unit_price_amount_snapshot": line.price,
			"quantity": line.input.Quantity, "line_total_amount": line.lineTotal,
			"perfume_id": nullableInt(input.PerfumeID), "perfume_name_snapshot": perfumeSnapshotString(perfumeSnapshot),
		})
	}
	auditValues, err := json.Marshal(map[string]any{
		"invoice_number": invoiceNumber, "customer_id": input.CustomerID, "total_amount": total.Int64(),
		"due_at": input.DueAt, "item_count": len(lines), "items": auditItems,
		"perfume_id": nullableInt(input.PerfumeID), "status": "RECEIVED", "payment_status": "UNPAID",
	})
	if err != nil {
		return OrderCreationResult{}, err
	}
	userAgent := actor.UserAgent
	if len(userAgent) > 512 {
		userAgent = userAgent[:512]
	}
	if err := q.InsertOrderAuditLog(ctx, postgresql.InsertOrderAuditLogParams{
		BusinessID: actor.BusinessID, OutletID: pgtype.Int8{Int64: input.OutletID, Valid: true},
		ActorUserID: pgtype.Int8{Int64: actor.UserID, Valid: true}, OrderID: pgtype.Int8{Int64: orderRow.ID, Valid: true},
		NewValues: auditValues, IpAddress: actor.IP, UserAgent: pgtype.Text{String: userAgent, Valid: userAgent != ""},
	}); err != nil {
		return OrderCreationResult{}, err
	}
	order, err := loadOrder(ctx, q, orderRow.ID, orderRow.BusinessID, orderRow.OutletID, orderRow.CustomerID,
		orderRow.InvoiceNumber, orderRow.Status, orderRow.PaymentStatus, orderRow.TotalAmount, orderRow.Notes,
		orderRow.ReceivedAt, orderRow.DueAt, orderRow.CreatedBy, orderRow.CreatedAt, orderRow.UpdatedAt)
	if err != nil {
		return OrderCreationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return OrderCreationResult{}, err
	}
	return OrderCreationResult{Order: order}, nil
}

func (s *OrderCatalog) Get(ctx context.Context, actor Actor, orderID int64) (Order, error) {
	row, err := s.database.Queries().GetOrder(ctx, postgresql.GetOrderParams{
		BusinessID: actor.BusinessID, OrderID: orderID, IsAdmin: actor.Role == "ADMIN", OutletIds: actor.OutletIDs,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, err
	}
	return loadOrder(ctx, s.database.Queries(), row.ID, row.BusinessID, row.OutletID, row.CustomerID,
		row.InvoiceNumber, row.Status, row.PaymentStatus, row.TotalAmount, row.Notes, row.ReceivedAt,
		row.DueAt, row.CreatedBy, row.CreatedAt, row.UpdatedAt)
}

func allowedOrderTransition(from, to string) bool {
	switch from {
	case "RECEIVED":
		return to == "PROCESSING" || to == "CANCELLED"
	case "PROCESSING":
		return to == "READY_FOR_PICKUP" || to == "CANCELLED"
	case "READY_FOR_PICKUP":
		return to == "COMPLETED" || to == "CANCELLED"
	default:
		return false
	}
}

func (s *OrderCatalog) TransitionStatus(ctx context.Context, actor Actor, orderID int64, toStatus string, notes *string) (OrderLifecycleResult, error) {
	if orderID < 1 || actor.BusinessID < 1 || actor.UserID < 1 || !validStatusTarget(toStatus) {
		return OrderLifecycleResult{}, ErrInvalidOrderTransition
	}
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return OrderLifecycleResult{}, err
	}
	defer tx.Rollback(ctx)
	q := s.database.Queries().WithTx(tx)
	before, err := lockLifecycleOrder(ctx, q, actor, orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrderLifecycleResult{}, ErrOrderNotFound
	}
	if err != nil {
		return OrderLifecycleResult{}, err
	}
	if !allowedOrderTransition(before.Status, toStatus) {
		return OrderLifecycleResult{}, ErrInvalidOrderTransition
	}
	updated, err := q.TransitionOrderStatus(ctx, postgresql.TransitionOrderStatusParams{BusinessID: actor.BusinessID, OrderID: orderID, ToStatus: toStatus})
	if err != nil {
		return OrderLifecycleResult{}, err
	}
	if err := q.InsertOrderStatusHistory(ctx, postgresql.InsertOrderStatusHistoryParams{
		BusinessID: actor.BusinessID, OutletID: before.OutletID, OrderID: orderID,
		FromStatus: pgtype.Text{String: before.Status, Valid: true}, ToStatus: toStatus, ChangedBy: actor.UserID, Notes: optionalText(notes),
	}); err != nil {
		return OrderLifecycleResult{}, err
	}
	oldValues, _ := json.Marshal(map[string]any{"status": before.Status, "completed_at": timePointer(before.CompletedAt)})
	newValues, _ := json.Marshal(map[string]any{"status": updated.Status, "completed_at": timePointer(updated.CompletedAt), "notes": notes})
	if err := insertLifecycleAudit(ctx, q, actor, before.OutletID, orderID, "ORDER_STATUS_CHANGED", oldValues, newValues); err != nil {
		return OrderLifecycleResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return OrderLifecycleResult{}, err
	}
	return OrderLifecycleResult{OrderID: orderID, Status: updated.Status, PaymentStatus: updated.PaymentStatus,
		UpdatedAt: updated.UpdatedAt.Time, CompletedAt: timePointer(updated.CompletedAt)}, nil
}

func validStatusTarget(status string) bool {
	return status == "PROCESSING" || status == "READY_FOR_PICKUP" || status == "COMPLETED"
}

func lockLifecycleOrder(ctx context.Context, q *postgresql.Queries, actor Actor, orderID int64) (postgresql.GetOrderForUpdateRow, error) {
	order, err := q.GetOrderForUpdate(ctx, postgresql.GetOrderForUpdateParams{BusinessID: actor.BusinessID, OrderID: orderID, IsAdmin: actor.Role == "ADMIN", UserID: actor.UserID})
	if err != nil {
		return postgresql.GetOrderForUpdateRow{}, err
	}
	if actor.Role != "ADMIN" {
		if _, err := q.LockCurrentOrderOutletAssignment(ctx, postgresql.LockCurrentOrderOutletAssignmentParams{BusinessID: actor.BusinessID, UserID: actor.UserID, OutletID: order.OutletID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return postgresql.GetOrderForUpdateRow{}, ErrOrderOutletForbidden
			}
			return postgresql.GetOrderForUpdateRow{}, err
		}
	}
	return order, nil
}

func (s *OrderCatalog) Cancel(ctx context.Context, actor Actor, orderID int64, reason string) (OrderLifecycleResult, error) {
	reason = strings.TrimSpace(reason)
	if orderID < 1 || actor.BusinessID < 1 || actor.UserID < 1 || reason == "" || len(reason) > 2000 {
		return OrderLifecycleResult{}, ErrInvalidCancellation
	}
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return OrderLifecycleResult{}, err
	}
	defer tx.Rollback(ctx)
	q := s.database.Queries().WithTx(tx)
	before, err := lockLifecycleOrder(ctx, q, actor, orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrderLifecycleResult{}, ErrOrderNotFound
	}
	if err != nil {
		return OrderLifecycleResult{}, err
	}
	if !allowedOrderTransition(before.Status, "CANCELLED") {
		return OrderLifecycleResult{}, ErrInvalidOrderTransition
	}
	if before.PaymentStatus != "UNPAID" {
		return OrderLifecycleResult{}, ErrOrderRequiresRefund
	}
	updated, err := q.CancelOrder(ctx, postgresql.CancelOrderParams{BusinessID: actor.BusinessID, OrderID: orderID,
		ActorUserID: pgtype.Int8{Int64: actor.UserID, Valid: true}, Reason: pgtype.Text{String: reason, Valid: true}})
	if err != nil {
		return OrderLifecycleResult{}, err
	}
	if err := q.InsertOrderStatusHistory(ctx, postgresql.InsertOrderStatusHistoryParams{
		BusinessID: actor.BusinessID, OutletID: before.OutletID, OrderID: orderID,
		FromStatus: pgtype.Text{String: before.Status, Valid: true}, ToStatus: "CANCELLED", ChangedBy: actor.UserID,
		Notes: pgtype.Text{String: reason, Valid: true},
	}); err != nil {
		return OrderLifecycleResult{}, err
	}
	oldValues, _ := json.Marshal(map[string]any{"status": before.Status, "payment_status": before.PaymentStatus})
	newValues, _ := json.Marshal(map[string]any{"status": updated.Status, "payment_status": updated.PaymentStatus,
		"cancelled_at": updated.CancelledAt.Time, "cancelled_by": actor.UserID, "cancellation_reason": reason, "deleted_at": updated.DeletedAt.Time})
	if err := insertLifecycleAudit(ctx, q, actor, before.OutletID, orderID, "ORDER_CANCELLED", oldValues, newValues); err != nil {
		return OrderLifecycleResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return OrderLifecycleResult{}, err
	}
	return OrderLifecycleResult{OrderID: orderID, Status: updated.Status, PaymentStatus: updated.PaymentStatus,
		UpdatedAt: updated.UpdatedAt.Time, CancelledAt: timePointer(updated.CancelledAt), CancellationReason: textPointer(updated.CancellationReason)}, nil
}

func (s *OrderCatalog) StatusHistory(ctx context.Context, actor Actor, orderID int64) ([]OrderStatusHistoryEntry, error) {
	if orderID < 1 || actor.BusinessID < 1 || actor.UserID < 1 {
		return nil, ErrOrderNotFound
	}
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	q := s.database.Queries().WithTx(tx)
	order, err := lockLifecycleOrder(ctx, q, actor, orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := q.ListOrderStatusHistory(ctx, postgresql.ListOrderStatusHistoryParams{BusinessID: actor.BusinessID, OutletID: order.OutletID, OrderID: orderID})
	if err != nil {
		return nil, err
	}
	result := make([]OrderStatusHistoryEntry, 0, len(rows))
	for _, row := range rows {
		result = append(result, OrderStatusHistoryEntry{ID: row.ID, FromStatus: textPointer(row.FromStatus), ToStatus: row.ToStatus, ChangedBy: row.ChangedBy, ChangedByName: row.ChangedByName, Notes: textPointer(row.Notes), ChangedAt: row.ChangedAt.Time})
	}
	return result, nil
}

func insertLifecycleAudit(ctx context.Context, q *postgresql.Queries, actor Actor, outletID, orderID int64, action string, oldValues, newValues []byte) error {
	userAgent := actor.UserAgent
	if len(userAgent) > 512 {
		userAgent = userAgent[:512]
	}
	return q.InsertOrderLifecycleAuditLog(ctx, postgresql.InsertOrderLifecycleAuditLogParams{BusinessID: actor.BusinessID,
		OutletID: pgtype.Int8{Int64: outletID, Valid: true}, ActorUserID: pgtype.Int8{Int64: actor.UserID, Valid: true}, Action: action,
		OrderID: pgtype.Int8{Int64: orderID, Valid: true}, OldValues: oldValues, NewValues: newValues,
		IpAddress: actor.IP, UserAgent: pgtype.Text{String: userAgent, Valid: userAgent != ""}})
}

func (s *OrderCatalog) List(ctx context.Context, actor Actor, filter OrderFilter) ([]OrderSummary, int64, error) {
	outletID := pgtype.Int8{}
	if filter.OutletID != nil {
		outletID = pgtype.Int8{Int64: *filter.OutletID, Valid: true}
	}
	status := pgtype.Text{}
	if filter.Status != nil {
		status = pgtype.Text{String: *filter.Status, Valid: true}
	}
	params := postgresql.ListOrdersParams{BusinessID: actor.BusinessID, IsAdmin: actor.Role == "ADMIN", OutletIds: actor.OutletIDs,
		OutletID: outletID, Status: status, PageOffset: filter.PageOffset, PageLimit: filter.PageLimit}
	rows, err := s.database.Queries().ListOrders(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.database.Queries().CountOrders(ctx, postgresql.CountOrdersParams{
		BusinessID: actor.BusinessID, IsAdmin: actor.Role == "ADMIN", OutletIds: actor.OutletIDs, OutletID: outletID, Status: status,
	})
	if err != nil {
		return nil, 0, err
	}
	result := make([]OrderSummary, 0, len(rows))
	for _, row := range rows {
		result = append(result, orderSummaryFrom(row.ID, row.BusinessID, row.OutletID, row.CustomerID, row.InvoiceNumber,
			row.Status, row.PaymentStatus, row.TotalAmount, row.Notes, row.ReceivedAt, row.DueAt, row.CreatedBy, row.CreatedAt, row.UpdatedAt))
	}
	return result, total, nil
}

func loadOrder(ctx context.Context, q *postgresql.Queries, id, businessID, outletID, customerID int64,
	invoiceNumber, status, paymentStatus string, totalAmount int64, notes pgtype.Text,
	receivedAt, dueAt pgtype.Timestamptz, createdBy int64, createdAt, updatedAt pgtype.Timestamptz) (Order, error) {
	items, err := q.ListOrderItems(ctx, postgresql.ListOrderItemsParams{BusinessID: businessID, OutletID: outletID, OrderID: id})
	if err != nil {
		return Order{}, err
	}
	order := Order{ID: id, BusinessID: businessID, OutletID: outletID, CustomerID: customerID,
		InvoiceNumber: invoiceNumber, Status: status, PaymentStatus: paymentStatus, TotalAmount: totalAmount,
		Notes: textPointer(notes), ReceivedAt: receivedAt.Time, DueAt: timePointer(dueAt), CreatedBy: createdBy,
		CreatedAt: createdAt.Time, UpdatedAt: updatedAt.Time, Items: make([]OrderItem, 0, len(items))}
	for _, item := range items {
		order.Items = append(order.Items, OrderItem{ID: item.ID, ServiceID: item.ServiceID,
			PerfumeID: intPointer(item.PerfumeID), ServiceNameSnapshot: item.ServiceNameSnapshot,
			ServiceUnitSnapshot: item.ServiceUnitSnapshot, UnitPriceAmountSnapshot: item.UnitPriceAmountSnapshot,
			PerfumeNameSnapshot: textPointer(item.PerfumeNameSnapshot), Quantity: item.Quantity,
			LineTotalAmount: item.LineTotalAmount, Notes: textPointer(item.Notes)})
	}
	return order, nil
}

func orderSummaryFrom(id, businessID, outletID, customerID int64, invoice, status, paymentStatus string,
	total int64, notes pgtype.Text, received, due pgtype.Timestamptz, createdBy int64,
	created, updated pgtype.Timestamptz) OrderSummary {
	return OrderSummary{ID: id, BusinessID: businessID, OutletID: outletID, CustomerID: customerID,
		InvoiceNumber: invoice, Status: status, PaymentStatus: paymentStatus, TotalAmount: total, Notes: textPointer(notes),
		ReceivedAt: received.Time, DueAt: timePointer(due), CreatedBy: createdBy, CreatedAt: created.Time, UpdatedAt: updated.Time}
}

func parseMilliQuantity(value string) (int64, error) {
	if value == "" || strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") || strings.Count(value, ".") > 1 {
		return 0, ErrInvalidOrder
	}
	parts := strings.SplitN(value, ".", 2)
	if len(parts[0]) == 0 || len(parts[0]) > 9 {
		return 0, ErrInvalidOrder
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, ErrInvalidOrder
	}
	for _, r := range parts[0] {
		if r < '0' || r > '9' {
			return 0, ErrInvalidOrder
		}
	}
	fraction := int64(0)
	if len(parts) == 2 {
		if len(parts[1]) == 0 || len(parts[1]) > 3 {
			return 0, ErrInvalidOrder
		}
		for _, r := range parts[1] {
			if r < '0' || r > '9' {
				return 0, ErrInvalidOrder
			}
		}
		padded := parts[1] + strings.Repeat("0", 3-len(parts[1]))
		fraction, err = strconv.ParseInt(padded, 10, 64)
		if err != nil {
			return 0, ErrInvalidOrder
		}
	}
	milli := whole*1000 + fraction
	if milli <= 0 || milli > 999999999999 {
		return 0, ErrInvalidOrder
	}
	return milli, nil
}

func containsOutlet(outlets []int64, outletID int64) bool {
	for _, id := range outlets {
		if id == outletID {
			return true
		}
	}
	return false
}

func nullableInt(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func intPointer(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}

func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func perfumeSnapshotString(value pgtype.Text) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func timePointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

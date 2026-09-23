package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository/postgresql"
)

type ReceiptsHandler struct{ database *repository.Postgres }

func NewReceiptsHandler(database *repository.Postgres) *ReceiptsHandler {
	return &ReceiptsHandler{database: database}
}

var receiptPlaceholder = regexp.MustCompile(`\{\{([^{}]+)\}\}`)
var receiptMalformedPlaceholder = regexp.MustCompile(`\{\{|\}\}`)

var receiptPlaceholders = map[string]bool{
	"customer_name": true, "invoice_number": true, "order_total": true,
	"paid_amount": true, "outstanding_amount": true, "due_date": true,
	"order_status": true,
}

func validateReceiptMessageTemplate(s string) error {
	for _, match := range receiptPlaceholder.FindAllStringSubmatch(s, -1) {
		if !receiptPlaceholders[match[1]] {
			return fmt.Errorf("unsupported placeholder: %s", match[1])
		}
	}
	stripped := receiptPlaceholder.ReplaceAllString(s, "")
	if receiptMalformedPlaceholder.MatchString(stripped) {
		return errors.New("malformed placeholder")
	}
	return nil
}

func renderReceiptMessage(template string, values map[string]string) string {
	return receiptPlaceholder.ReplaceAllStringFunc(template, func(token string) string {
		key := receiptPlaceholder.FindStringSubmatch(token)[1]
		return values[key]
	})
}

func rupiah(n int64) string {
	s := strconv.FormatInt(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "." + s[i:]
	}
	return "Rp " + s
}

type receiptTemplateOutput struct {
	ID                      int64   `json:"id"`
	OutletID                int64   `json:"outletId"`
	Name                    string  `json:"name"`
	HeaderText              *string `json:"headerText"`
	FooterText              *string `json:"footerText"`
	WhatsAppMessageTemplate *string `json:"whatsappMessageTemplate"`
	PaperWidthMM            int32   `json:"paperWidthMm"`
	IsDefault               bool    `json:"isDefault"`
	CreatedAt               any     `json:"createdAt"`
	UpdatedAt               any     `json:"updatedAt"`
}

func receiptTemplate(id, outlet int64, name string, header, footer, message pgtype.Text, width int32, def bool, created, updated any) receiptTemplateOutput {
	return receiptTemplateOutput{ID: id, OutletID: outlet, Name: name, HeaderText: textPtr(header), FooterText: textPtr(footer), WhatsAppMessageTemplate: textPtr(message), PaperWidthMM: width, IsDefault: def, CreatedAt: created, UpdatedAt: updated}
}

func textPtr(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func (h *ReceiptsHandler) receipt(c echo.Context, p Principal, orderID pgtype.Int8, outletID pgtype.Int8, qrID pgtype.Text) error {
	q := h.database.Queries()
	ctx := c.Request().Context()
	order, err := q.GetAuthorizedReceiptOrder(ctx, postgresql.GetAuthorizedReceiptOrderParams{UserID: p.UserID, IsAdmin: p.Role == "ADMIN", BusinessID: p.BusinessID, OutletID: outletID, OrderID: orderID, QrID: qrID})
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	}
	if err != nil {
		return internalError(c)
	}
	items, err := q.ListOrderItems(ctx, postgresql.ListOrderItemsParams{BusinessID: p.BusinessID, OutletID: order.OutletID, OrderID: order.ID})
	if err != nil {
		return internalError(c)
	}
	paid, err := q.GetReceiptConfirmedPaymentTotal(ctx, postgresql.GetReceiptConfirmedPaymentTotalParams{BusinessID: p.BusinessID, OutletID: order.OutletID, OrderID: order.ID})
	if err != nil {
		return internalError(c)
	}
	var due any
	dueText := ""
	if order.DueAt.Valid {
		due = order.DueAt.Time
		dueText = order.DueAt.Time.Format("2006-01-02")
	}
	message := (*string)(nil)
	if order.WhatsappMessageTemplate.Valid {
		outstanding := order.TotalAmount - paid
		if outstanding < 0 {
			outstanding = 0
		}
		rendered := renderReceiptMessage(order.WhatsappMessageTemplate.String, map[string]string{
			"customer_name": order.CustomerNameSnapshot, "invoice_number": order.InvoiceNumber,
			"order_total": rupiah(order.TotalAmount), "paid_amount": rupiah(paid),
			"outstanding_amount": rupiah(outstanding), "due_date": dueText, "order_status": order.Status,
		})
		message = &rendered
	}
	type item struct {
		ID          int64   `json:"id"`
		ServiceName string  `json:"serviceName"`
		Unit        string  `json:"unit"`
		UnitPrice   int64   `json:"unitPriceAmount"`
		Quantity    string  `json:"quantity"`
		LineTotal   int64   `json:"lineTotalAmount"`
		PerfumeName *string `json:"perfumeName"`
		Notes       *string `json:"notes"`
	}
	lineItems := make([]item, 0, len(items))
	for _, v := range items {
		lineItems = append(lineItems, item{ID: v.ID, ServiceName: v.ServiceNameSnapshot, Unit: v.ServiceUnitSnapshot, UnitPrice: v.UnitPriceAmountSnapshot, Quantity: v.Quantity, LineTotal: v.LineTotalAmount, PerfumeName: textPtr(v.PerfumeNameSnapshot), Notes: textPtr(v.Notes)})
	}
	outstanding := order.TotalAmount - paid
	if outstanding < 0 {
		outstanding = 0
	}
	return c.JSON(http.StatusOK, map[string]any{
		"order":  map[string]any{"id": order.ID, "invoiceNumber": order.InvoiceNumber, "status": order.Status, "receivedAt": order.ReceivedAt.Time, "dueAt": due, "customer": map[string]any{"name": order.CustomerNameSnapshot, "phone": textPtr(order.CustomerPhoneSnapshot)}, "totalAmount": order.TotalAmount, "receiptQrId": order.ReceiptQrID},
		"outlet": map[string]any{"id": order.OutletID, "code": order.OutletCode, "name": order.OutletName, "phone": textPtr(order.OutletPhone), "address": textPtr(order.OutletAddress)},
		"items":  lineItems, "payment": map[string]any{"status": order.PaymentStatus, "paidAmount": paid, "outstandingAmount": outstanding},
		"template":      map[string]any{"id": nullableInt(order.ReceiptTemplateID), "name": textPtr(order.ReceiptTemplateName), "headerText": textPtr(order.ReceiptHeaderText), "footerText": textPtr(order.ReceiptFooterText), "paperWidthMm": defaultWidth(order.PaperWidthMm)},
		"whatsappShare": map[string]any{"phone": textPtr(order.CustomerPhoneSnapshot), "message": message},
	})
}

func nullableInt(v pgtype.Int8) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}
func defaultWidth(v pgtype.Int4) int32 {
	if !v.Valid {
		return 58
	}
	return v.Int32
}

func (h *ReceiptsHandler) GetOrderReceipt(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	id, err := pathID(c, "orderId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Order ID must be a positive integer.")
	}
	return h.receipt(c, p, pgtype.Int8{Int64: id, Valid: true}, pgtype.Int8{}, pgtype.Text{})
}

func (h *ReceiptsHandler) GetReceiptByQR(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	outlet, err := pathID(c, "outletId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Outlet ID must be a positive integer.")
	}
	qr := c.Param("qrId")
	if len(qr) != 36 || !regexp.MustCompile(`^[0-9a-fA-F-]{36}$`).MatchString(qr) {
		return notFound(c)
	}
	return h.receipt(c, p, pgtype.Int8{}, pgtype.Int8{Int64: outlet, Valid: true}, pgtype.Text{String: qr, Valid: true})
}

type receiptTemplateInput struct {
	Name                    *string `json:"name"`
	HeaderText              *string `json:"headerText"`
	FooterText              *string `json:"footerText"`
	WhatsAppMessageTemplate *string `json:"whatsappMessageTemplate"`
	PaperWidthMM            *int32  `json:"paperWidthMm"`
	IsDefault               *bool   `json:"isDefault"`
}

func (in receiptTemplateInput) validate(update bool) error {
	if in.Name == nil || strings.TrimSpace(*in.Name) == "" || len(*in.Name) > 120 {
		return errors.New("name is required and must not exceed 120 characters")
	}
	for _, v := range []*string{in.HeaderText, in.FooterText} {
		if v != nil && len(*v) > 2000 {
			return errors.New("headerText and footerText must not exceed 2000 characters")
		}
	}
	if in.WhatsAppMessageTemplate != nil {
		if len(*in.WhatsAppMessageTemplate) > 4000 {
			return errors.New("whatsappMessageTemplate must not exceed 4000 characters")
		}
		if err := validateReceiptMessageTemplate(*in.WhatsAppMessageTemplate); err != nil {
			return err
		}
	}
	if in.PaperWidthMM == nil || (*in.PaperWidthMM != 58 && *in.PaperWidthMM != 80) {
		return errors.New("paperWidthMm must be 58 or 80")
	}
	if update && in.IsDefault == nil {
		return errors.New("isDefault is required for updates")
	}
	return nil
}

func nullableText(v *string) pgtype.Text {
	return pgtype.Text{String: func() string {
		if v == nil {
			return ""
		}
		return *v
	}(), Valid: v != nil}
}

func (h *ReceiptsHandler) ListTemplates(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	outlet, err := pathID(c, "outletId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Outlet ID must be a positive integer.")
	}
	ctx := c.Request().Context()
	q := h.database.Queries()
	if _, err = q.GetAuthorizedReceiptOutlet(ctx, postgresql.GetAuthorizedReceiptOutletParams{UserID: p.UserID, IsAdmin: p.Role == "ADMIN", BusinessID: p.BusinessID, OutletID: outlet}); errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	} else if err != nil {
		return internalError(c)
	}
	rows, err := q.ListReceiptTemplates(ctx, postgresql.ListReceiptTemplatesParams{BusinessID: p.BusinessID, OutletID: outlet})
	if err != nil {
		return internalError(c)
	}
	items := make([]receiptTemplateOutput, 0, len(rows))
	for _, v := range rows {
		items = append(items, receiptTemplate(v.ID, v.OutletID, v.Name, v.HeaderText, v.FooterText, v.WhatsappMessageTemplate, v.PaperWidthMm, v.IsDefault, v.CreatedAt.Time, v.UpdatedAt.Time))
	}
	return c.JSON(http.StatusOK, map[string]any{"items": items})
}

func (h *ReceiptsHandler) GetTemplate(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	outlet, err := pathID(c, "outletId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Outlet ID must be a positive integer.")
	}
	id, err := pathID(c, "templateId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Template ID must be a positive integer.")
	}
	ctx := c.Request().Context()
	q := h.database.Queries()
	if _, err = q.GetAuthorizedReceiptOutlet(ctx, postgresql.GetAuthorizedReceiptOutletParams{UserID: p.UserID, IsAdmin: p.Role == "ADMIN", BusinessID: p.BusinessID, OutletID: outlet}); errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	} else if err != nil {
		return internalError(c)
	}
	v, err := q.GetReceiptTemplateForUpdate(ctx, postgresql.GetReceiptTemplateForUpdateParams{BusinessID: p.BusinessID, OutletID: outlet, TemplateID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	}
	if err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusOK, receiptTemplate(v.ID, v.OutletID, v.Name, v.HeaderText, v.FooterText, v.WhatsappMessageTemplate, v.PaperWidthMm, v.IsDefault, v.CreatedAt.Time, v.UpdatedAt.Time))
}

func (h *ReceiptsHandler) mutateTemplate(c echo.Context, p Principal, outlet int64, id int64, in receiptTemplateInput, action string) (receiptTemplateOutput, error) {
	ctx := c.Request().Context()
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return receiptTemplateOutput{}, err
	}
	defer tx.Rollback(ctx)
	q := postgresql.New(tx)
	if _, err = q.LockReceiptTemplateOutlet(ctx, postgresql.LockReceiptTemplateOutletParams{UserID: p.UserID, IsAdmin: p.Role == "ADMIN", BusinessID: p.BusinessID, OutletID: outlet}); err != nil {
		return receiptTemplateOutput{}, err
	}
	var old any
	if action != "CREATE" {
		v, e := q.GetReceiptTemplateForUpdate(ctx, postgresql.GetReceiptTemplateForUpdateParams{BusinessID: p.BusinessID, OutletID: outlet, TemplateID: id})
		if e != nil {
			return receiptTemplateOutput{}, e
		}
		old = mapTemplateAudit(v.Name, v.HeaderText, v.FooterText, v.WhatsappMessageTemplate, v.PaperWidthMm, v.IsDefault)
	}
	def := false
	if in.IsDefault != nil {
		def = *in.IsDefault
	} else {
		has, e := q.HasActiveDefaultReceiptTemplate(ctx, postgresql.HasActiveDefaultReceiptTemplateParams{BusinessID: p.BusinessID, OutletID: outlet})
		if e != nil {
			return receiptTemplateOutput{}, e
		}
		def = !has
	}
	if def {
		if err = q.ClearActiveDefaultReceiptTemplates(ctx, postgresql.ClearActiveDefaultReceiptTemplatesParams{BusinessID: p.BusinessID, OutletID: outlet}); err != nil {
			return receiptTemplateOutput{}, err
		}
	}
	width := int32(58)
	if in.PaperWidthMM != nil {
		width = *in.PaperWidthMM
	}
	var v receiptTemplateOutput
	if action == "CREATE" {
		row, e := q.CreateReceiptTemplate(ctx, postgresql.CreateReceiptTemplateParams{BusinessID: p.BusinessID, OutletID: outlet, Name: *in.Name, HeaderText: nullableText(in.HeaderText), FooterText: nullableText(in.FooterText), WhatsappMessageTemplate: nullableText(in.WhatsAppMessageTemplate), PaperWidthMm: width, IsDefault: def})
		err = e
		if e == nil {
			v = receiptTemplate(row.ID, row.OutletID, row.Name, row.HeaderText, row.FooterText, row.WhatsappMessageTemplate, row.PaperWidthMm, row.IsDefault, row.CreatedAt.Time, row.UpdatedAt.Time)
		}
	} else {
		row, e := q.UpdateReceiptTemplate(ctx, postgresql.UpdateReceiptTemplateParams{BusinessID: p.BusinessID, OutletID: outlet, TemplateID: id, Name: *in.Name, HeaderText: nullableText(in.HeaderText), FooterText: nullableText(in.FooterText), WhatsappMessageTemplate: nullableText(in.WhatsAppMessageTemplate), PaperWidthMm: width, IsDefault: def})
		err = e
		if e == nil {
			v = receiptTemplate(row.ID, row.OutletID, row.Name, row.HeaderText, row.FooterText, row.WhatsappMessageTemplate, row.PaperWidthMm, row.IsDefault, row.CreatedAt.Time, row.UpdatedAt.Time)
		}
	}
	if err != nil {
		return receiptTemplateOutput{}, err
	}
	newJSON, _ := json.Marshal(mapTemplateAudit(*in.Name, nullableText(in.HeaderText), nullableText(in.FooterText), nullableText(in.WhatsAppMessageTemplate), width, def))
	var oldJSON []byte
	if old != nil {
		oldJSON, _ = json.Marshal(old)
	}
	var ip *netip.Addr
	if addr, e := netip.ParseAddr(c.RealIP()); e == nil {
		ip = &addr
	}
	ua := c.Request().UserAgent()
	if len(ua) > 512 {
		ua = ua[:512]
	}
	if err = q.InsertReceiptTemplateAuditLog(ctx, postgresql.InsertReceiptTemplateAuditLogParams{BusinessID: p.BusinessID, OutletID: pgtype.Int8{Int64: outlet, Valid: true}, ActorUserID: pgtype.Int8{Int64: p.UserID, Valid: true}, Action: action, TemplateID: pgtype.Int8{Int64: v.ID, Valid: true}, OldValues: oldJSON, NewValues: newJSON, IpAddress: ip, UserAgent: pgtype.Text{String: ua, Valid: ua != ""}}); err != nil {
		return receiptTemplateOutput{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return receiptTemplateOutput{}, err
	}
	return v, nil
}

func mapTemplateAudit(name string, h, f, m pgtype.Text, w int32, d bool) map[string]any {
	return map[string]any{"name": name, "headerText": textPtr(h), "footerText": textPtr(f), "whatsappMessageTemplate": textPtr(m), "paperWidthMm": w, "isDefault": d}
}

func (h *ReceiptsHandler) CreateTemplate(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	outlet, err := pathID(c, "outletId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Outlet ID must be a positive integer.")
	}
	var in receiptTemplateInput
	if err = decodeManagementJSON(c, &in); err != nil {
		return badRequest(c, "INVALID_TEMPLATE", "Template body is invalid.")
	}
	if err = in.validate(false); err != nil {
		return badRequest(c, "INVALID_TEMPLATE", err.Error())
	}
	v, err := h.mutateTemplate(c, p, outlet, 0, in, "CREATE")
	if err != nil {
		return receiptTemplateError(c, err)
	}
	return c.JSON(http.StatusCreated, v)
}
func (h *ReceiptsHandler) UpdateTemplate(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	outlet, err := pathID(c, "outletId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Outlet ID must be a positive integer.")
	}
	id, err := pathID(c, "templateId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Template ID must be a positive integer.")
	}
	var in receiptTemplateInput
	if err = decodeManagementJSON(c, &in); err != nil {
		return badRequest(c, "INVALID_TEMPLATE", "Template body is invalid.")
	}
	if err = in.validate(true); err != nil {
		return badRequest(c, "INVALID_TEMPLATE", err.Error())
	}
	v, err := h.mutateTemplate(c, p, outlet, id, in, "UPDATE")
	if err != nil {
		return receiptTemplateError(c, err)
	}
	return c.JSON(http.StatusOK, v)
}
func (h *ReceiptsHandler) DeleteTemplate(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	outlet, err := pathID(c, "outletId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Outlet ID must be a positive integer.")
	}
	id, err := pathID(c, "templateId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Template ID must be a positive integer.")
	}
	ctx := c.Request().Context()
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := postgresql.New(tx)
	if _, err = q.LockReceiptTemplateOutlet(ctx, postgresql.LockReceiptTemplateOutletParams{UserID: p.UserID, IsAdmin: p.Role == "ADMIN", BusinessID: p.BusinessID, OutletID: outlet}); err != nil {
		return receiptTemplateError(c, err)
	}
	old, err := q.GetReceiptTemplateForUpdate(ctx, postgresql.GetReceiptTemplateForUpdateParams{BusinessID: p.BusinessID, OutletID: outlet, TemplateID: id})
	if err != nil {
		return receiptTemplateError(c, err)
	}
	deleted, err := q.SoftDeleteReceiptTemplate(ctx, postgresql.SoftDeleteReceiptTemplateParams{BusinessID: p.BusinessID, OutletID: outlet, TemplateID: id})
	if err != nil {
		return receiptTemplateError(c, err)
	}
	if old.IsDefault {
		if err = q.SetNextReceiptTemplateDefault(ctx, postgresql.SetNextReceiptTemplateDefaultParams{BusinessID: p.BusinessID, OutletID: outlet, ExcludeTemplateID: id}); err != nil {
			return internalError(c)
		}
	}
	oldJSON, _ := json.Marshal(mapTemplateAudit(old.Name, old.HeaderText, old.FooterText, old.WhatsappMessageTemplate, old.PaperWidthMm, old.IsDefault))
	var ip *netip.Addr
	if addr, e := netip.ParseAddr(c.RealIP()); e == nil {
		ip = &addr
	}
	ua := c.Request().UserAgent()
	if len(ua) > 512 {
		ua = ua[:512]
	}
	if err = q.InsertReceiptTemplateAuditLog(ctx, postgresql.InsertReceiptTemplateAuditLogParams{BusinessID: p.BusinessID, OutletID: pgtype.Int8{Int64: outlet, Valid: true}, ActorUserID: pgtype.Int8{Int64: p.UserID, Valid: true}, Action: "DELETE", TemplateID: pgtype.Int8{Int64: id, Valid: true}, OldValues: oldJSON, NewValues: []byte(`{"deleted":true}`), IpAddress: ip, UserAgent: pgtype.Text{String: ua, Valid: ua != ""}}); err != nil {
		return internalError(c)
	}
	if err = tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusOK, receiptTemplate(deleted.ID, deleted.OutletID, deleted.Name, deleted.HeaderText, deleted.FooterText, deleted.WhatsappMessageTemplate, deleted.PaperWidthMm, false, deleted.CreatedAt.Time, deleted.UpdatedAt.Time))
}

func receiptTemplateError(c echo.Context, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	}
	var pgerr interface{ SQLState() string }
	if errors.As(err, &pgerr) && pgerr.SQLState() == "23505" {
		return conflict(c, "TEMPLATE_CONFLICT", "An active receipt template with this name already exists.")
	}
	return internalError(c)
}

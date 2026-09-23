package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository/postgresql"
)

type DashboardHandler struct{ database *repository.Postgres }

func NewDashboardHandler(database *repository.Postgres) *DashboardHandler {
	return &DashboardHandler{database: database}
}

type dashboardOutlet struct {
	ID         int64     `json:"id"`
	BusinessID int64     `json:"business_id"`
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	Phone      *string   `json:"phone"`
	Address    *string   `json:"address"`
	IsActive   bool      `json:"is_active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type dashboardPayments struct {
	ConfirmedCount       int64 `json:"confirmed_count"`
	ConfirmedTotalAmount int64 `json:"confirmed_total_amount"`
}

type dashboardExpenses struct {
	Count       int64 `json:"count"`
	TotalAmount int64 `json:"total_amount"`
}

type dashboardResponse struct {
	Outlet       dashboardOutlet   `json:"outlet"`
	BusinessDate string            `json:"business_date"`
	Timezone     string            `json:"timezone"`
	Payments     dashboardPayments `json:"payments"`
	Expenses     dashboardExpenses `json:"expenses"`
}

func (h *DashboardHandler) GetOutlet(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	outletID, err := pathID(c, "outletId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Outlet ID must be a positive integer.")
	}
	row, err := h.database.Queries().GetOutletDashboard(c.Request().Context(), postgresql.GetOutletDashboardParams{
		UserID: p.UserID, IsAdmin: p.Role == "ADMIN", BusinessID: p.BusinessID, OutletID: outletID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	}
	if err != nil {
		return internalError(c)
	}
	var phone, address *string
	if row.Phone.Valid {
		value := row.Phone.String
		phone = &value
	}
	if row.Address.Valid {
		value := row.Address.String
		address = &value
	}
	date := row.BusinessDate.Time.Format("2006-01-02")
	return c.JSON(http.StatusOK, dashboardResponse{
		Outlet: dashboardOutlet{
			ID: row.OutletID, BusinessID: row.BusinessID, Code: row.Code, Name: row.Name,
			Phone: phone, Address: address, IsActive: row.IsActive,
			CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		},
		BusinessDate: date,
		Timezone:     row.Timezone,
		Payments: dashboardPayments{
			ConfirmedCount: row.ConfirmedPaymentCount, ConfirmedTotalAmount: row.ConfirmedPaymentAmount,
		},
		Expenses: dashboardExpenses{Count: row.ExpenseCount, TotalAmount: row.ExpenseAmount},
	})
}

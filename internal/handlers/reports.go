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

const maxReportRangeDays = 366

type ReportsHandler struct{ database *repository.Postgres }

func NewReportsHandler(database *repository.Postgres) *ReportsHandler {
	return &ReportsHandler{database: database}
}

// Get returns one of the fixed report kinds registered by the router. Report
// SQL returns a database-built JSON document so totals remain exact integers.
func (h *ReportsHandler) Get(reportType string) echo.HandlerFunc {
	return func(c echo.Context) error {
		p, ok := PrincipalFromContext(c)
		if !ok {
			return unauthorized(c)
		}
		outletID, err := pathID(c, "outletId")
		if err != nil {
			return badRequest(c, "INVALID_ID", "Outlet ID must be a positive integer.")
		}
		from, to, err := reportDateRange(c)
		if err != nil {
			return badRequest(c, "INVALID_DATE_RANGE", err.Error())
		}
		result, err := h.database.Queries().GetOutletReport(c.Request().Context(), postgresql.GetOutletReportParams{
			UserID: p.UserID, IsAdmin: p.Role == "ADMIN", BusinessID: p.BusinessID, OutletID: outletID,
			DateFrom: dateParam(from), DateTo: dateParam(to), ReportType: reportType,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound(c)
		}
		if err != nil {
			return internalError(c)
		}
		return c.JSONBlob(http.StatusOK, result)
	}
}

func reportDateRange(c echo.Context) (time.Time, time.Time, error) {
	from, to, err := expenseDateRange(c)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	days := int(to.Sub(from).Hours()/24) + 1
	if days > maxReportRangeDays {
		return time.Time{}, time.Time{}, errors.New("date range cannot exceed 366 local calendar days")
	}
	return from, to, nil
}

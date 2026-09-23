package handlers

import "github.com/labstack/echo/v4"

const principalContextKey = "auth.principal"

type Principal struct {
	UserID, BusinessID, SessionID int64
	Email, FullName, Role         string
	OutletIDs                     []int64
	Permissions                   []string
}

func PrincipalFromContext(c echo.Context) (Principal, bool) {
	p, ok := c.Get(principalContextKey).(Principal)
	return p, ok
}
func SetPrincipal(c echo.Context, p Principal) { c.Set(principalContextKey, p) }

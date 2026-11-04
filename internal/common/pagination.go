package common

import (
	"net/http"
	"strconv"
)

const (
	defaultPage    = 1
	defaultPerPage = 20
	maxPerPage     = 100
)

// PaginationParams holds parsed pagination query parameters.
type PaginationParams struct {
	Page    int
	PerPage int
}

// Offset returns the SQL OFFSET for the current page.
func (p PaginationParams) Offset() int {
	return (p.Page - 1) * p.PerPage
}

// ParsePagination reads ?page and ?per_page from the request, applying
// defaults and clamping per_page to maxPerPage.
func ParsePagination(r *http.Request) PaginationParams {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))

	if page < 1 {
		page = defaultPage
	}
	if perPage < 1 {
		perPage = defaultPerPage
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}

	return PaginationParams{Page: page, PerPage: perPage}
}

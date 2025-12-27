package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"message": "hello"}

	JSON(w, http.StatusOK, data)

	result := w.Result()
	if result.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, result.StatusCode)
	}

	if result.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", result.Header.Get("Content-Type"))
	}

	var response Response
	if err := json.NewDecoder(result.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.Success {
		t.Error("Response should be successful")
	}
}

func TestJSONWithMeta(t *testing.T) {
	w := httptest.NewRecorder()
	data := []string{"item1", "item2"}
	meta := &Meta{
		Total:      100,
		Page:       1,
		PerPage:    10,
		TotalPages: 10,
	}

	JSONWithMeta(w, http.StatusOK, data, meta)

	var response Response
	if err := json.NewDecoder(w.Result().Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Meta == nil {
		t.Fatal("Response should have meta")
	}

	if response.Meta.Total != 100 {
		t.Errorf("Expected total 100, got %d", response.Meta.Total)
	}
}

func TestError(t *testing.T) {
	w := httptest.NewRecorder()

	Error(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid input")

	result := w.Result()
	if result.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, result.StatusCode)
	}

	var response Response
	if err := json.NewDecoder(result.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Success {
		t.Error("Response should not be successful")
	}

	if response.Error == nil {
		t.Fatal("Response should have error")
	}

	if response.Error.Code != "BAD_REQUEST" {
		t.Errorf("Expected error code 'BAD_REQUEST', got '%s'", response.Error.Code)
	}

	if response.Error.Message != "Invalid input" {
		t.Errorf("Expected message 'Invalid input', got '%s'", response.Error.Message)
	}
}

func TestValidationErrorResponse(t *testing.T) {
	w := httptest.NewRecorder()
	errors := ValidationErrors{
		{Field: "name", Message: "is required"},
		{Field: "email", Message: "is invalid"},
	}

	ValidationErrorResponse(w, errors)

	result := w.Result()
	if result.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, result.StatusCode)
	}

	var response Response
	if err := json.NewDecoder(result.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("Expected error code 'VALIDATION_ERROR', got '%s'", response.Error.Code)
	}

	if len(response.Error.Details) != 2 {
		t.Errorf("Expected 2 error details, got %d", len(response.Error.Details))
	}
}

func TestBadRequest(t *testing.T) {
	w := httptest.NewRecorder()
	BadRequest(w, "Test error")

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status %d", http.StatusBadRequest)
	}
}

func TestNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	NotFound(w, "Not found")

	if w.Result().StatusCode != http.StatusNotFound {
		t.Errorf("Expected status %d", http.StatusNotFound)
	}
}

func TestInternalError(t *testing.T) {
	w := httptest.NewRecorder()
	InternalError(w, "Server error")

	if w.Result().StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status %d", http.StatusInternalServerError)
	}
}

func TestUnauthorized(t *testing.T) {
	w := httptest.NewRecorder()
	Unauthorized(w, "Not authenticated")

	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status %d", http.StatusUnauthorized)
	}
}

func TestForbidden(t *testing.T) {
	w := httptest.NewRecorder()
	Forbidden(w, "Access denied")

	if w.Result().StatusCode != http.StatusForbidden {
		t.Errorf("Expected status %d", http.StatusForbidden)
	}
}

func TestConflict(t *testing.T) {
	w := httptest.NewRecorder()
	Conflict(w, "Resource conflict")

	if w.Result().StatusCode != http.StatusConflict {
		t.Errorf("Expected status %d", http.StatusConflict)
	}
}

func TestCreated(t *testing.T) {
	w := httptest.NewRecorder()
	Created(w, map[string]string{"id": "123"})

	if w.Result().StatusCode != http.StatusCreated {
		t.Errorf("Expected status %d", http.StatusCreated)
	}
}

func TestOK(t *testing.T) {
	w := httptest.NewRecorder()
	OK(w, "success")

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("Expected status %d", http.StatusOK)
	}
}

func TestNoContent(t *testing.T) {
	w := httptest.NewRecorder()
	NoContent(w)

	if w.Result().StatusCode != http.StatusNoContent {
		t.Errorf("Expected status %d", http.StatusNoContent)
	}
}

func TestList(t *testing.T) {
	w := httptest.NewRecorder()
	items := []string{"a", "b", "c"}

	List(w, items, 100, 2, 10)

	var response Response
	if err := json.NewDecoder(w.Result().Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Meta == nil {
		t.Fatal("List response should have meta")
	}

	if response.Meta.Total != 100 {
		t.Errorf("Expected total 100, got %d", response.Meta.Total)
	}

	if response.Meta.Page != 2 {
		t.Errorf("Expected page 2, got %d", response.Meta.Page)
	}

	if response.Meta.PerPage != 10 {
		t.Errorf("Expected perPage 10, got %d", response.Meta.PerPage)
	}

	if response.Meta.TotalPages != 10 {
		t.Errorf("Expected totalPages 10, got %d", response.Meta.TotalPages)
	}
}

func TestListTotalPagesCalculation(t *testing.T) {
	tests := []struct {
		total       int
		perPage     int
		expectedPages int
	}{
		{100, 10, 10},
		{101, 10, 11},
		{99, 10, 10},
		{0, 10, 0},
		{5, 10, 1},
	}

	for _, tc := range tests {
		w := httptest.NewRecorder()
		List(w, []string{}, tc.total, 1, tc.perPage)

		var response Response
		_ = json.NewDecoder(w.Result().Body).Decode(&response)

		if response.Meta.TotalPages != tc.expectedPages {
			t.Errorf("total=%d, perPage=%d: expected pages %d, got %d",
				tc.total, tc.perPage, tc.expectedPages, response.Meta.TotalPages)
		}
	}
}

func TestListDefaultsForZeroValues(t *testing.T) {
	// Test that List handles zero values for page and perPage
	w := httptest.NewRecorder()
	List(w, []string{"a", "b"}, 50, 0, 0)

	var response Response
	_ = json.NewDecoder(w.Result().Body).Decode(&response)

	if response.Meta == nil {
		t.Fatal("Response should have meta")
	}

	// Should default to page 1 and perPage 10
	if response.Meta.Page != 1 {
		t.Errorf("Expected page 1, got %d", response.Meta.Page)
	}
	if response.Meta.PerPage != 10 {
		t.Errorf("Expected perPage 10, got %d", response.Meta.PerPage)
	}
	if response.Meta.TotalPages != 5 {
		t.Errorf("Expected totalPages 5, got %d", response.Meta.TotalPages)
	}
}

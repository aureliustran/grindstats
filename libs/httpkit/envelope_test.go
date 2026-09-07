package httpkit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"grindstats/libs/auditmodel"
	"grindstats/libs/i18n"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestContext(acceptLanguage string) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	if acceptLanguage != "" {
		c.Request.Header.Set("Accept-Language", acceptLanguage)
	}
	return c, rec
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) errorEnvelope {
	t.Helper()
	var env errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return env
}

func TestError_WritesStatusAndCodeFromRegistry(t *testing.T) {
	c, rec := newTestContext("")

	Error(c, auditmodel.ErrServiceUnavailable)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	env := decodeEnvelope(t, rec)
	if env.Error.Code != auditmodel.ErrServiceUnavailable {
		t.Errorf("code = %q, want %q", env.Error.Code, auditmodel.ErrServiceUnavailable)
	}
}

func TestError_MessageRendersInRequestLocale(t *testing.T) {
	c, rec := newTestContext("vi-VN")

	Error(c, auditmodel.ErrServiceUnavailable)

	env := decodeEnvelope(t, rec)
	want := i18n.Render(string(auditmodel.ErrServiceUnavailable), "vi-VN")
	if env.Error.Message != want {
		t.Errorf("message = %q, want %q", env.Error.Message, want)
	}
	if env.Error.Message == i18n.Render(string(auditmodel.ErrServiceUnavailable), "en-US") {
		t.Error("vi-VN request produced the en-US message")
	}
}

func TestError_CodeIsIdenticalAcrossLocales(t *testing.T) {
	// The three-planes rule at the envelope boundary: locale changes the
	// message, never the code a client branches on.
	viCtx, viRec := newTestContext("vi-VN")
	Error(viCtx, auditmodel.ErrServiceUnavailable)

	enCtx, enRec := newTestContext("en-US")
	Error(enCtx, auditmodel.ErrServiceUnavailable)

	viEnv := decodeEnvelope(t, viRec)
	enEnv := decodeEnvelope(t, enRec)

	if viEnv.Error.Code != enEnv.Error.Code {
		t.Errorf("code differs across locales: vi-VN=%q en-US=%q", viEnv.Error.Code, enEnv.Error.Code)
	}
	if viEnv.Error.Message == enEnv.Error.Message {
		t.Error("messages are identical across locales; catalogs should differ")
	}
}

func TestError_UndeclaredCodePanics(t *testing.T) {
	c, _ := newTestContext("")

	defer func() {
		if recover() == nil {
			t.Fatal("Error did not panic on an undeclared code")
		}
	}()

	Error(c, auditmodel.ErrorCode("NOT_A_REAL_CODE"))
}

func TestInternalError_WritesInternalErrorCode(t *testing.T) {
	c, rec := newTestContext("")

	InternalError(c)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	env := decodeEnvelope(t, rec)
	if env.Error.Code != auditmodel.ErrInternalError {
		t.Errorf("code = %q, want %q", env.Error.Code, auditmodel.ErrInternalError)
	}
}

func TestServiceUnavailable_NamesNoDependencyInBody(t *testing.T) {
	c, rec := newTestContext("")

	ServiceUnavailable(c)

	body := strings.ToLower(rec.Body.String())
	for _, leaky := range []string{"postgres", "redis", "pgx", "tcp", "dial"} {
		if strings.Contains(body, leaky) {
			t.Errorf("body %q leaks dependency detail %q", rec.Body.String(), leaky)
		}
	}
}

func TestStatusFor_KnownAndUnknownCodes(t *testing.T) {
	if got := StatusFor(auditmodel.ErrServiceUnavailable); got != http.StatusServiceUnavailable {
		t.Errorf("StatusFor(ServiceUnavailable) = %d, want %d", got, http.StatusServiceUnavailable)
	}
	if got := StatusFor(auditmodel.ErrorCode("NOT_A_REAL_CODE")); got != http.StatusInternalServerError {
		t.Errorf("StatusFor(unknown) = %d, want %d", got, http.StatusInternalServerError)
	}
}

type widget struct {
	Name string `json:"name"`
}

func TestOK_WritesDataEnvelopeWith200(t *testing.T) {
	c, rec := newTestContext("")

	OK(c, widget{Name: "wrench"})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body struct {
		Data widget `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if body.Data.Name != "wrench" {
		t.Errorf("data.name = %q, want %q", body.Data.Name, "wrench")
	}
}

func TestCreated_WritesDataEnvelopeWith201(t *testing.T) {
	c, rec := newTestContext("")

	Created(c, widget{Name: "hammer"})

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if !strings.Contains(rec.Body.String(), `"hammer"`) {
		t.Errorf("body %q does not contain the created resource", rec.Body.String())
	}
}

func TestData_WritesGivenStatus(t *testing.T) {
	c, rec := newTestContext("")

	Data(c, http.StatusAccepted, widget{Name: "drill"})

	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
}

func TestPaginated_WritesDataAndPaginationEnvelope(t *testing.T) {
	c, rec := newTestContext("")

	items := []widget{{Name: "a"}, {Name: "b"}}
	Paginated(c, items, NewPagination(1, 2, 5))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body struct {
		Data       []widget   `json:"data"`
		Pagination Pagination `json:"pagination"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if len(body.Data) != 2 {
		t.Errorf("len(data) = %d, want 2", len(body.Data))
	}
	want := Pagination{Page: 1, PageSize: 2, TotalItems: 5, TotalPages: 3}
	if body.Pagination != want {
		t.Errorf("pagination = %+v, want %+v", body.Pagination, want)
	}
}

func TestPaginated_NilItemsWritesEmptyArrayNotNull(t *testing.T) {
	c, rec := newTestContext("")

	Paginated[widget](c, nil, NewPagination(1, 10, 0))

	if strings.Contains(rec.Body.String(), `"data":null`) {
		t.Errorf("body %q has null data, want []", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Errorf("body %q does not have an empty data array", rec.Body.String())
	}
}

func TestNewPagination_ComputesTotalPagesRoundingUp(t *testing.T) {
	cases := []struct {
		page, pageSize, totalItems int
		wantTotalPages             int
	}{
		{1, 10, 0, 0},
		{1, 10, 10, 1},
		{1, 10, 11, 2},
		{1, 10, 25, 3},
	}
	for _, tc := range cases {
		got := NewPagination(tc.page, tc.pageSize, tc.totalItems)
		if got.TotalPages != tc.wantTotalPages {
			t.Errorf("NewPagination(%d, %d, %d).TotalPages = %d, want %d",
				tc.page, tc.pageSize, tc.totalItems, got.TotalPages, tc.wantTotalPages)
		}
	}
}

func TestNewPagination_NonPositivePageSizeAvoidsDivideByZero(t *testing.T) {
	got := NewPagination(1, 0, 100)
	if got.TotalPages != 0 {
		t.Errorf("TotalPages = %d, want 0 for pageSize=0", got.TotalPages)
	}
}

package kellyframework

import (
	"context"
	"fmt"
	"github.com/julienschmidt/httprouter"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type empty struct {}

type validatorEnabled struct {
	A int `validate:"required"`
}

type dummyMethodCallLogger struct{}

var e = empty{}
var dummyLogger = &dummyMethodCallLogger{}

func (l *dummyMethodCallLogger) Record(field string, value string) {}

func (e *empty) errorMethod(*ServiceMethodContext, *empty) error {
	return fmt.Errorf("expected error")
}

func (e *empty) errorResponseMethod(*ServiceMethodContext, *empty) *FormattedResponse {
	return &FormattedResponse{403, "forbidden", nil}
}

func (e *empty) panicMethod(*ServiceMethodContext, *empty) interface{} {
	panic("expected panic")
	return nil
}

func normalFunc(*ServiceMethodContext, *struct{}) *struct{ A int } {
	return &struct{ A int }{1}
}

func nilFunc(*ServiceMethodContext, *struct{}) *struct{ A int } {
	return nil
}

func validatorEnabledFunc(*ServiceMethodContext, *validatorEnabled) error {
	return nil
}

func TestServiceHandlerCheckServiceMethodPrototype(t *testing.T) {
	t.Run("not function", func(t *testing.T) {
		if err := checkServiceMethodPrototype(reflect.TypeOf(1)); err == nil {
			t.Error()
		}
	})

	t.Run("arguments count wrong", func(t *testing.T) {
		if err := checkServiceMethodPrototype(reflect.TypeOf(func() {})); err == nil {
			t.Error()
		}
	})

	t.Run("first argument type wrong", func(t *testing.T) {
		if err := checkServiceMethodPrototype(reflect.TypeOf(func(*struct{}, *struct{}) {})); err == nil {
			t.Error()
		}
	})

	t.Run("second argument type wrong", func(t *testing.T) {
		if err := checkServiceMethodPrototype(reflect.TypeOf(func(*ServiceMethodContext, struct{}) {})); err == nil {
			t.Error()
		}
		if err := checkServiceMethodPrototype(reflect.TypeOf(func(*ServiceMethodContext, []struct{}) {})); err == nil {
			t.Error()
		}
	})

	t.Run("return values count wrong", func(t *testing.T) {
		if err := checkServiceMethodPrototype(reflect.TypeOf(func(*ServiceMethodContext, *struct{}) (int, int) {return 0, 0})); err == nil {
			t.Error()
		}
	})

	t.Run("normal method", func(t *testing.T) {
		if err := checkServiceMethodPrototype(reflect.TypeOf(e.errorMethod)); err != nil {
			t.Error()
		}
	})
}

func TestServiceHandlerServeHTTP(t *testing.T) {
	normalFuncHandler, _ := NewServiceHandler(normalFunc, "kftest", false, false)
	nilFuncHandler, _ := NewServiceHandler(nilFunc, "kftest", false, true)
	errorMethodHandler, _ := NewServiceHandler(e.errorMethod, "kftest", false, false)
	errorRespMethodHandler, _ := NewServiceHandler(e.errorResponseMethod, "kftest", false, false)
	panicMethodHandler, _ := NewServiceHandler(e.panicMethod, "kftest", false, false)
	validatorEnabledFuncHandler, _ := NewServiceHandler(validatorEnabledFunc, "kftest", false, false)

	t.Run("empty function with empty json", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader("{}"))
		req = req.WithContext(context.WithValue(req.Context(), "kftest", dummyLogger))
		req.Header.Add("content-type", "application/json")
		recorder := httptest.NewRecorder()
		normalFuncHandler.ServeHTTP(recorder, req)
		if recorder.Code != 200 {
			t.Error("code is not 200, body:", recorder.Body)
		}
	})

	t.Run("nil function with empty json", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader("{}"))
		req = req.WithContext(context.WithValue(req.Context(), "kftest", dummyLogger))
		req.Header.Add("content-type", "application/json")
		recorder := httptest.NewRecorder()
		nilFuncHandler.ServeHTTP(recorder, req)
		if recorder.Code != 200 {
			t.Error("code is not 200, body:", recorder.Body)
		}
	})

	t.Run("empty function with syntax error json", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader("{312}"))
		req = req.WithContext(context.WithValue(req.Context(), "kftest", dummyLogger))
		req.Header.Add("content-type", "application/json")
		recorder := httptest.NewRecorder()
		normalFuncHandler.ServeHTTP(recorder, req)
		if recorder.Code != 400 {
			t.Error("code is not 400, body:", recorder.Body)
		}
	})

	t.Run("empty function with empty string", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader(""))
		req = req.WithContext(context.WithValue(req.Context(), "kftest", dummyLogger))
		req.Header.Add("content-type", "application/json")
		recorder := httptest.NewRecorder()
		normalFuncHandler.ServeHTTP(recorder, req)
		if recorder.Code != 400 {
			t.Error("code is not 400, body:", recorder.Body)
		}
	})

	t.Run("error method with empty json", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader("{}"))
		req = req.WithContext(context.WithValue(req.Context(), "kftest", dummyLogger))
		req.Header.Add("content-type", "application/json")
		recorder := httptest.NewRecorder()
		errorMethodHandler.ServeHTTP(recorder, req)
		if recorder.Code != 500 {
			t.Error("code is not 500, body:", recorder.Body)
		}
	})

	t.Run("error response method with empty json", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader("{}"))
		req = req.WithContext(context.WithValue(req.Context(), "kftest", dummyLogger))
		req.Header.Add("content-type", "application/json")
		recorder := httptest.NewRecorder()
		errorRespMethodHandler.ServeHTTP(recorder, req)
		if recorder.Code != 403 {
			t.Error("code is not 403, body:", recorder.Body)
		}
	})

	t.Run("panic method with empty json", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader("{}"))
		req = req.WithContext(context.WithValue(req.Context(), "kftest", dummyLogger))
		req.Header.Add("content-type", "application/json")
		recorder := httptest.NewRecorder()
		panicMethodHandler.ServeHTTP(recorder, req)
		if recorder.Code != 500 {
			t.Error("code is not 500, body:", recorder.Body)
		}
	})

	t.Run("validator enabled function with normal json", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader("{\"A\": 1, \"B\":2}"))
		req = req.WithContext(context.WithValue(req.Context(), "kftest", dummyLogger))
		req.Header.Add("content-type", "application/json")
		recorder := httptest.NewRecorder()
		validatorEnabledFuncHandler.ServeHTTPWithParams(recorder, req, httprouter.Params{httprouter.Param{Key: "A", Value: "2"}})
		if recorder.Code != 200 {
			t.Error("code is not 200, body:", recorder.Body)
		}
	})

	t.Run("validator enabled function with normal query string", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/?a=1&b=2", strings.NewReader("{}"))
		req = req.WithContext(context.WithValue(req.Context(), "kftest", dummyLogger))
		req.Header.Add("content-type", "application/json")
		recorder := httptest.NewRecorder()
		validatorEnabledFuncHandler.ServeHTTP(recorder, req)
		if recorder.Code != 200 {
			t.Error("code is not 200, body:", recorder.Body)
		}
	})

	t.Run("validator enabled function with invalid json", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader("{}"))
		req = req.WithContext(context.WithValue(req.Context(), "kftest", dummyLogger))
		req.Header.Add("content-type", "application/json")
		recorder := httptest.NewRecorder()
		validatorEnabledFuncHandler.ServeHTTP(recorder, req)
		if recorder.Code != 400 {
			t.Error("code is not 400, body:", recorder.Body, ", code:", recorder.Code)
		}
	})

	t.Run("validator enabled function with invalid query string", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/?b=1", strings.NewReader("{}"))
		req = req.WithContext(context.WithValue(req.Context(), "kftest", dummyLogger))
		recorder := httptest.NewRecorder()
		validatorEnabledFuncHandler.ServeHTTP(recorder, req)
		if recorder.Code != 400 {
			t.Error("code is not 400, body:", recorder.Body, ", code:", recorder.Code)
		}
	})
}

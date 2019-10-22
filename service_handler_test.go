package kellyframework

import (
	"fmt"
	"github.com/julienschmidt/httprouter"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type empty struct {
}

type validatorEnabled struct {
	A int `validate:"required"`
}

var e = empty{}

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

func emptyFunction(*ServiceMethodContext, *empty) *struct{ A int } {
	return &struct{ A int }{1}
}

func validatorEnabledFunction(*ServiceMethodContext, *validatorEnabled) error {
	return nil
}

func TestServiceHandlerCheckServiceMethodPrototype(t *testing.T) {
	t.Run("not function", func(t *testing.T) {
		if err := checkServiceMethodPrototype(reflect.TypeOf(1)); err == nil {
			t.Error()
		}
	})

	t.Run("argument count wrong", func(t *testing.T) {
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

	t.Run("return value count wrong", func(t *testing.T) {
		if err := checkServiceMethodPrototype(reflect.TypeOf(func(*ServiceMethodContext, *struct{}) {})); err == nil {
			t.Error()
		}
	})

	t.Run("two return values second type wrong", func(t *testing.T) {
		if err := checkServiceMethodPrototype(reflect.TypeOf(func(*ServiceMethodContext, *struct{}) (int, int) { return 0, 0 })); err == nil {
			t.Error()
		}
	})

	t.Run("normal function", func(t *testing.T) {
		if err := checkServiceMethodPrototype(reflect.TypeOf(emptyFunction)); err != nil {
			t.Error()
		}
	})

	t.Run("normal object method", func(t *testing.T) {
		if err := checkServiceMethodPrototype(reflect.TypeOf(emptyFunction)); err != nil {
			t.Error()
		}
	})

	t.Run("normal object method", func(t *testing.T) {
		if err := checkServiceMethodPrototype(reflect.TypeOf(e.errorMethod)); err != nil {
			t.Error()
		}
	})
}

func TestServiceHandlerServeHTTP(t *testing.T) {
	h1, _ := NewServiceHandler(emptyFunction, nil, false, false)
	h2, _ := NewServiceHandler(e.errorMethod, nil, false, false)
	h3, _ := NewServiceHandler(e.errorResponseMethod, nil, false, false)
	h4, _ := NewServiceHandler(e.panicMethod, nil, false, false)
	h5, _ := NewServiceHandler(validatorEnabledFunction, nil, false, false)

	emptyFunctionNormalArguments := httptest.NewRequest("POST", "/emptyFunction", strings.NewReader("{}"))
	emptyFunctionNormalArguments.Header.Add("content-type", "application/json")
	t.Run("normal arguments", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		h1.ServeHTTP(recorder, emptyFunctionNormalArguments)
		if recorder.Code != 200 {
			t.Error("code is not 200, body:", recorder.Body)
		}
	})

	emptyFunctionWrongArguments := httptest.NewRequest("POST", "/emptyFunction", strings.NewReader("{312}"))
	emptyFunctionWrongArguments.Header.Add("content-type", "application/json")
	t.Run("wrong arguments", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		h1.ServeHTTP(recorder, emptyFunctionWrongArguments)
		if recorder.Code != 400 {
			t.Error("code is not 400, body:", recorder.Body)
		}
	})

	emptyFunctionEmptyArguments := httptest.NewRequest("POST", "/emptyFunction", strings.NewReader(""))
	emptyFunctionEmptyArguments.Header.Add("content-type", "application/json")
	t.Run("empty arguments", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		h1.ServeHTTP(recorder, emptyFunctionEmptyArguments)
		if recorder.Code != 400 {
			t.Error("code is not 400, body:", recorder.Body)
		}
	})

	errorMethodNormalArguments := httptest.NewRequest("POST", "/errorMethod", strings.NewReader("{}"))
	errorMethodNormalArguments.Header.Add("content-type", "application/json")
	t.Run("error", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		h2.ServeHTTP(recorder, errorMethodNormalArguments)
		if recorder.Code != 500 {
			t.Error("code is not 500, body:", recorder.Body)
		}
	})

	errorResponseMethodNormalArguments := httptest.NewRequest("POST", "/errorMethod", strings.NewReader("{}"))
	errorResponseMethodNormalArguments.Header.Add("content-type", "application/json")
	t.Run("error", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		h3.ServeHTTP(recorder, errorResponseMethodNormalArguments)
		if recorder.Code != 403 {
			t.Error("code is not 403, body:", recorder.Body)
		}
	})

	panicMethodNormalArguments := httptest.NewRequest("POST", "/panicMethod", strings.NewReader("{}"))
	panicMethodNormalArguments.Header.Add("content-type", "application/json")
	t.Run("panic", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		h4.ServeHTTP(recorder, panicMethodNormalArguments)
		if recorder.Code != 500 {
			t.Error("code is not 500, body:", recorder.Body)
		}
	})

	validatorEnabledFunctionNormalArguments := httptest.NewRequest("POST", "/validatorEnabledFunction", strings.NewReader("{\"A\": 1, \"B\":2}"))
	validatorEnabledFunctionNormalArguments.Header.Add("content-type", "application/json")
	t.Run("validator enabled normal arguments", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		h5.ServeHTTPWithParams(recorder, validatorEnabledFunctionNormalArguments, httprouter.Params{httprouter.Param{"A", "2"}})
		if recorder.Code != 200 {
			t.Error("code is not 200, body:", recorder.Body)
		}
	})

	validatorEnabledFunctionNormalQueryString := httptest.NewRequest("POST", "/validatorEnabledFunction?a=1&b=2", strings.NewReader("{}"))
	validatorEnabledFunctionNormalQueryString.Header.Add("content-type", "application/json")
	t.Run("validator enabled invalid arguments", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		h5.ServeHTTP(recorder, validatorEnabledFunctionNormalQueryString)
		if recorder.Code != 200 {
			t.Error("code is not 200, body:", recorder.Body)
		}
	})

	validatorEnabledFunctionInvalidArguments := httptest.NewRequest("POST", "/validatorEnabledFunction", strings.NewReader("{}"))
	validatorEnabledFunctionInvalidArguments.Header.Add("content-type", "application/json")
	t.Run("validator enabled invalid arguments", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		h5.ServeHTTP(recorder, validatorEnabledFunctionInvalidArguments)
		if recorder.Code != 400 {
			t.Error("code is not 400, body:", recorder.Body, ", code:", recorder.Code)
		}
	})

	validatorEnabledFunctionInvalidQueryString := httptest.NewRequest("POST", "/validatorEnabledFunction?b=1", strings.NewReader("{}"))
	t.Run("validator enabled invalid arguments", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		h5.ServeHTTP(recorder, validatorEnabledFunctionInvalidQueryString)
		if recorder.Code != 400 {
			t.Error("code is not 400, body:", recorder.Body, ", code:", recorder.Code)
		}
	})
}

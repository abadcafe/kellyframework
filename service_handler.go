package kellyframework

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/schema"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/trace"
	"gopkg.in/go-playground/validator.v9"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

type ServiceMethodContext struct {
	Context            context.Context
	RemoteAddr         string
	RequestHeader      http.Header
	RequestBodyReader  io.ReadCloser
	ResponseHeader     http.Header
	ResponseBodyWriter io.Writer
}

type MethodCallLogger interface {
	Record(field string, value string)
}

type ServiceHandler struct {
	loggerContextKey   interface{}
	method             *serviceMethod
	validator          *validator.Validate
	bypassRequestBody  bool
	bypassResponseBody bool
}

type FormattedResponse struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

type serviceMethod struct {
	value   reflect.Value
	argType reflect.Type
}

type panicStack struct {
	Panic string `json:"panic"`
	Stack string `json:"stack"`
}

const traceFamily = "kellyframework.ServiceHandler"

var formDecoder = schema.NewDecoder()

func init() {
	formDecoder.IgnoreUnknownKeys(true)
}

func checkServiceMethodPrototype(methodType reflect.Type) error {
	if methodType.Kind() != reflect.Func {
		return fmt.Errorf("you should provide a function or object method")
	}

	if methodType.NumIn() != 2 {
		return fmt.Errorf("the service method should have two arguments")
	}

	if methodType.In(0).Kind() != reflect.Ptr || methodType.In(0).Elem().Name() != "ServiceMethodContext" {
		return fmt.Errorf("the first argument should be type *ServiceMethodContext")
	}

	if methodType.In(1).Kind() != reflect.Ptr ||
		(methodType.In(1).Elem().Kind() != reflect.Struct && methodType.In(1).Elem().Kind() != reflect.Slice) {
		return fmt.Errorf("the second argument should be a struct pointer or slice pointer")
	}

	if methodType.NumOut() != 1 {
		return fmt.Errorf("the service method should have only one return value")
	}

	return nil
}

func NewServiceHandler(method interface{}, loggerContextKey interface{}, bypassRequestBody bool,
	bypassResponseBody bool) (h *ServiceHandler, err error) {
	// the method prototype like this: 'func(*ServiceMethodContext, *struct) (anything)'
	methodType := reflect.TypeOf(method)
	err = checkServiceMethodPrototype(methodType)
	if err != nil {
		return
	}

	h = &ServiceHandler{
		loggerContextKey,
		&serviceMethod{
			reflect.ValueOf(method),
			methodType.In(1),
		},
		validator.New(),
		bypassRequestBody,
		bypassResponseBody,
	}

	return
}

func setResponseHeader(w http.ResponseWriter) {
	// Prevents Internet Explorer from MIME-sniffing a response away from the declared content-type
	w.Header().Set("x-content-type-options", "nosniff")
	w.Header().Set("Content-Type", "application/json")
}

func writeResponse(w http.ResponseWriter, tr trace.Trace, data interface{}) {
	tr.LazyPrintf("%+v", data)
	setResponseHeader(w)
	json.NewEncoder(w).Encode(data)
}

func writeFormattedResponse(w http.ResponseWriter, tr trace.Trace, resp *FormattedResponse) {
	tr.LazyPrintf("%s: %+v", resp.Msg, resp.Data)
	if resp.Code >= 400 {
		tr.SetError()
	}

	setResponseHeader(w)
	w.WriteHeader(resp.Code)
	json.NewEncoder(w).Encode(resp)
}

func doServiceMethodCall(method *serviceMethod, in []reflect.Value) (out []reflect.Value, ps *panicStack) {
	defer func() {
		if panicInfo := recover(); panicInfo != nil {
			ps = &panicStack{
				fmt.Sprintf("%s", panicInfo),
				fmt.Sprintf("%s", debug.Stack()),
			}
		}
	}()

	out = method.value.Call(in)
	return
}

func (h *ServiceHandler) parseArgument(r *http.Request, params httprouter.Params, arg interface{}) error {
	// query string has lowest priority.
	err := r.ParseForm()
	if err != nil {
		return err
	}

	a := reflect.ValueOf(arg)
	if a.Elem().Kind() == reflect.Struct {
		err = formDecoder.Decode(arg, r.Form)
		if err != nil {
			return err
		}
	}

	// json content is prior to query string.
	if !h.bypassRequestBody && strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		err := json.NewDecoder(r.Body).Decode(arg)
		if err != nil {
			return err
		}
	}

	// params is prior to json content.
	if params != nil {
		paramValues := url.Values{}
		for _, param := range params {
			paramValues.Set(param.Key, param.Value)
		}

		err = formDecoder.Decode(arg, paramValues)
		if err != nil {
			return err
		}
	}

	if a.Elem().Kind() == reflect.Struct {
		err = h.validator.Struct(arg)
		if err != nil {
			return err
		}
	}
	
	return nil
}

func (h *ServiceHandler) ServeHTTP(respWriter http.ResponseWriter, req *http.Request) {
	h.ServeHTTPWithParams(respWriter, req, nil)
}

func (h *ServiceHandler) ServeHTTPWithParams(rw http.ResponseWriter, r *http.Request, params httprouter.Params) {
	tracer := trace.New(traceFamily, r.URL.Path)
	defer tracer.Finish()

	// extract arguments.
	arg := reflect.New(h.method.argType.Elem())
	err := h.parseArgument(r, params, arg.Interface())
	if err != nil {
		writeFormattedResponse(rw, tracer, &FormattedResponse{400, "parse argument failed", err.Error()})
		return
	}

	// do method call.
	beginTime := time.Now()
	out, methodPanic := doServiceMethodCall(h.method, []reflect.Value{
		reflect.ValueOf(&ServiceMethodContext{
			r.Context(),
			r.RemoteAddr,
			r.Header,
			r.Body,
			rw.Header(),
			rw,
		}),
		arg,
	})
	duration := time.Now().Sub(beginTime)

	// write returned value or error to response.
	if methodPanic == nil && len(out) != 1 {
		// the method prototype have more than one return value, it is forbidden.
		panic(fmt.Sprintf("return values error: %+v", out))
	}

	var respData interface{}
	if methodPanic != nil {
		respData = &FormattedResponse{500, "service method panicked", methodPanic}
		writeFormattedResponse(rw, tracer, respData.(*FormattedResponse))
	} else {
		methodReturn := out[0].Interface()
		ok := false
		if respData, ok = methodReturn.(*FormattedResponse); ok {
			if respData.(*FormattedResponse) != nil {
				writeFormattedResponse(rw, tracer, respData.(*FormattedResponse))
			}
		} else if err, ok = methodReturn.(error); ok {
			respData = &FormattedResponse{500, "service method error", err.Error()}
			writeFormattedResponse(rw, tracer, respData.(*FormattedResponse))
		} else if !h.bypassResponseBody {
			// write to response body as JSON encoded string
			respData = methodReturn
			writeResponse(rw, tracer, respData)
		}
	}

	// record some thing if logger existed.
	if h.loggerContextKey != nil {
		logger := r.Context().Value(h.loggerContextKey).(MethodCallLogger)
		if logger != nil {
			marshaledArgs, err := json.Marshal(arg.Interface())
			if err != nil {
				panic(err)
			}

			marshaledData, err := json.Marshal(respData)
			if err != nil {
				panic(err)
			}

			logger.Record("methodCallArgument", string(marshaledArgs))
			logger.Record("methodCallResponseData", string(marshaledData))
			logger.Record("methodCallBeginTime", beginTime.Format("2006-01-02 15:04:05.999999999"))
			logger.Record("methodCallDuration", strconv.FormatFloat(duration.Seconds(), 'f', -1, 64))
		}
	}
}

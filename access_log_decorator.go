package kellyframework

import (
	"net/http"
	"context"
	"time"
	"io"
	"github.com/sirupsen/logrus"
	"strconv"
	"encoding/json"
)

type AccessLogDecorator struct {
	http.Handler
	loggingHeaders      []string
	rowFillerContextKey interface{}
	rowFillerFactory    AccessLogRowFillerFactory
	logger              *logrus.Logger
}

type AccessLogRow struct {
	fields logrus.Fields
}

type AccessLogRowFiller interface{}
type AccessLogRowFillerFactory func(*AccessLogRow) AccessLogRowFiller

func (row *AccessLogRow) SetRowField(field string, value string) {
	row.fields[field] = value
}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func NewAccessLogDecorator(handler http.Handler, logWriter io.Writer, loggingHeaders []string,
	rowFillerContextKey interface{}, rowFillerFactory AccessLogRowFillerFactory) *AccessLogDecorator {
	logger := logrus.New()
	logger.Formatter = &logrus.TextFormatter{DisableTimestamp: true}
	logger.Out = logWriter
	return &AccessLogDecorator{
		handler,
		loggingHeaders,
		rowFillerContextKey,
		rowFillerFactory,
		logger,
	}
}

func (d *AccessLogDecorator) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	beginTime := time.Now()
	row := &AccessLogRow{
		make(logrus.Fields),
	}

	if d.rowFillerContextKey != nil && d.rowFillerFactory != nil {
		rowFiller := d.rowFillerFactory(row)
		r = r.WithContext(context.WithValue(r.Context(), d.rowFillerContextKey, rowFiller))
	}

	sw := &statusResponseWriter{
		w,
		http.StatusOK,
	}

	d.Handler.ServeHTTP(sw, r)

	headers := make(map[string][]string)
	for _, k := range d.loggingHeaders {
		headers[k] = r.Header[k]
	}
	marshaledHeaders, err := json.Marshal(headers)
	if err != nil {
		panic(err)
	}

	row.SetRowField("beginTime", beginTime.Format("2006-01-02 15:04:05.999999999"))
	row.SetRowField("status", strconv.Itoa(sw.status))
	row.SetRowField("duration", strconv.FormatFloat(time.Now().Sub(beginTime).Seconds(), 'f', -1, 64))
	row.SetRowField("remote", r.RemoteAddr)
	row.SetRowField("httpMethod", r.Method)
	row.SetRowField("uri", r.URL.RequestURI())
	row.SetRowField("headers", string(marshaledHeaders))

	if sw.status < http.StatusBadRequest {
		d.logger.WithFields(row.fields).Info()
	} else {
		d.logger.WithFields(row.fields).Error()
	}
}

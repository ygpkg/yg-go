package server

import (
	"encoding/json"
	"net/http"
	"reflect"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/apiobj"
	"github.com/ygpkg/yg-go/apis/constants"
	"github.com/ygpkg/yg-go/apis/errcode"
	"github.com/ygpkg/yg-go/apis/runtime"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
)

// handleAPI .
func transAPI(hdr interface{}) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		hdrType := reflect.TypeOf(hdr)
		// logs.Debug("hdrtype ", hdrType.Kind())
		switch hdrType.Kind() {
		case reflect.Func:
			// logs.Debug("hdrtype num ", hdrType.NumIn(), hdrType.NumOut())
			if hdrType.NumIn() != 3 {
				logs.Error("invalid handler params number.", hdrType.NumIn())
				runtime.InternalError(ctx, "invalid handler params number. %T %v", hdr, hdrType.NumIn())
				return
			}
			if hdrType.NumOut() > 1 {
				logs.Error("invalid handler returns number.")
				runtime.InternalError(ctx, "invalid handler returns number. %T %v", hdr, hdrType.NumOut())
				return
			}

			inVal := reflect.New(hdrType.In(1).Elem())
			outVal := reflect.New(hdrType.In(2).Elem())
			{
				in := inVal.Interface()
				err := json.NewDecoder(ctx.Request.Body).Decode(in)
				if err != nil {
					logs.Errorf("decode request failed, %s", err)
					runtime.BadRequest(ctx, "decode request failed, %s", err)
					return
				}
				// inVal.FieldByName("Version").String()
			}

			vals := reflect.ValueOf(hdr).Call([]reflect.Value{
				reflect.ValueOf(ctx),
				inVal,
				outVal,
			})
			if len(vals) > 0 {
				retVal := vals[0].Interface()
				if retVal != nil {
					err, ok := retVal.(error)
					if !ok {
						logs.Errorf("invalid handler returns type: %T", retVal)
						runtime.InternalError(ctx, "invalid handler returns type: %T", retVal)
						return
					}
					if err != nil {
						logs.Errorf("handler return error: %s", err)
						runtime.InternalError(ctx, "handler return error %T %s", retVal, err)
						return
					}
				}
			}
			if ctx.IsAborted() {
				return
			}

			fixBaseResponse(ctx, outVal)
			out := outVal.Interface()

			ctx.JSON(http.StatusOK, out)

		default:
			logs.Error("failed hdrType")
			runtime.BadRequest(ctx, "failed hdrType %T", hdr)
			return
		}
	}
}

func transHttp(hdr http.HandlerFunc) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		hdr(ctx.Writer, ctx.Request)
	}
}

func transHttpHdr(hdr http.Handler) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		hdr.ServeHTTP(ctx.Writer, ctx.Request)
	}
}

// fixBaseResponse 填充基础返回信息
func fixBaseResponse(ctx *gin.Context, val reflect.Value) {
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() == reflect.Struct {
		for i := 0; i < val.NumField(); i++ {
			field := val.Field(i)
			if field.Kind() == reflect.Ptr {
				field = field.Elem()
			}

			fieldName := val.Type().Field(i).Name
			if fieldName == "Code" && field.CanInt() {
				ctx.Set(constants.CtxKeyCode, int(field.Int()))
				return
			}

			if field.Type() == reflect.TypeOf(apiobj.BaseResponse{}) {
				br := field.Interface().(apiobj.BaseResponse)
				if br.Message == "" {
					br.Message = errcode.GetMessage(br.Code)
				}
				ctx.Set(constants.CtxKeyCode, int(br.Code))
				br.RequestID = ctx.GetString(constants.CtxKeyRequestID)
				br.Env = config.Conf().MainConf.Env
				field.Set(reflect.ValueOf(br))
				return
			}
		}
	}
}

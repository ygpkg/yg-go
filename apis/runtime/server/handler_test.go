package server

import (
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/apiobj"
)

func TestFixBaseResponse1(t *testing.T) {
	testCtx := gin.CreateTestContextOnly(nil, gin.Default())
	resp := &apiobj.BaseResponse{
		Code:    1,
		Message: "test",
	}

	val := reflect.ValueOf(resp)
	fixBaseResponse(testCtx, val)
	code := testCtx.GetInt("code")
	if code != 1 {
		t.Errorf("code should be 1, but got %d", code)
	}
}

func TestFixBaseResponse2(t *testing.T) {
	testCtx := gin.CreateTestContextOnly(nil, gin.Default())
	type TestResponse struct {
		apiobj.BaseResponse
		Response struct{}
	}
	resp := &TestResponse{
		BaseResponse: apiobj.BaseResponse{
			Code:    2,
			Message: "test",
		},
	}

	val := reflect.ValueOf(resp)
	fixBaseResponse(testCtx, val)
	code := testCtx.GetInt("code")
	if code != 2 {
		t.Errorf("code should be 2, but got %d", code)
	}
}

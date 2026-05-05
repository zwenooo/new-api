package relay

import (
	"io"
	"one-api/common"
	"one-api/types"

	"github.com/gin-gonic/gin"
)

func newRequestBodyReadError(err error) *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		err,
		types.ErrorCodeReadRequestBodyFailed,
		common.RequestBodyErrorStatusCode(err),
		types.ErrOptionWithSkipRetry(),
	)
}

func getPassThroughRequestBody(c *gin.Context) (io.Reader, *types.NewAPIError) {
	bodyStorage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, newRequestBodyReadError(err)
	}
	return common.NewReusableBodyReader(bodyStorage), nil
}

package classroom

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGetFreeClassRoomReqValidation(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{
			name:  "valid",
			query: "year=2024-2025&semester=2&week=6&day=1&sections=1&sections=2&wherePrefix=n1",
		},
		{
			name:    "missing sections",
			query:   "year=2024-2025&semester=2&week=6&day=1&wherePrefix=n1",
			wantErr: true,
		},
		{
			name:    "invalid semester",
			query:   "year=2024-2025&semester=4&week=6&day=1&sections=1&wherePrefix=n1",
			wantErr: true,
		},
		{
			name:    "invalid section",
			query:   "year=2024-2025&semester=2&week=6&day=1&sections=13&wherePrefix=n1",
			wantErr: true,
		},
	}

	gin.SetMode(gin.TestMode)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest("GET", "/classroom/getFreeClassRoom?"+tt.query, nil)

			var req GetFreeClassRoomReq
			err := ctx.ShouldBindQuery(&req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validation error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

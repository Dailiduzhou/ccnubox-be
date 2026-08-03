package http

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/asynccnu/ccnubox-be/be-class/biz/model"
	"github.com/stretchr/testify/assert"
	"github.com/xuri/excelize/v2"
)

// 模拟的 FreeClassRoomSaver 实现，用于测试
type MockFreeClassRoomBiz struct {
	year     string
	semester string
	ctwPairs []model.CTWPair
}

func (m *MockFreeClassRoomBiz) SaveFreeClassRoomInfo(ctx context.Context, year, semester string, cwtPairs []model.CTWPair) error {
	m.year = year
	m.semester = semester
	m.ctwPairs = append([]model.CTWPair(nil), cwtPairs...)
	return nil
}

func TestUploadSelection(t *testing.T) {
	// 模拟上传的 JSON 数据
	jsonData := `{
		"year": "2024",
		"semester": "2",
		"sheets": {
			"2024级": {
				"class_time_idx": 7,
				"class_where_idx": 8
			}
		}
	}`

	excelFile := excelize.NewFile()
	defer excelFile.Close()
	const sheet = "2024级"
	_, err := excelFile.NewSheet(sheet)
	if err != nil {
		t.Fatalf("Failed to create worksheet: %v", err)
	}
	if err := excelFile.SetCellValue(sheet, "H2", "星期一第1-2节{1-2周}"); err != nil {
		t.Fatal(err)
	}
	if err := excelFile.SetCellValue(sheet, "I2", "N101"); err != nil {
		t.Fatal(err)
	}
	excelBytes, err := excelFile.WriteToBuffer()
	if err != nil {
		t.Fatalf("Failed to create test Excel data: %v", err)
	}

	// 创建请求体（multipart/form-data）
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	// 添加 JSON 数据部分
	part, err := writer.CreateFormField("json_data")
	if err != nil {
		t.Fatalf("Failed to create form field for json_data: %v", err)
	}
	part.Write([]byte(jsonData))

	// 添加文件部分
	filePart, err := writer.CreateFormFile("file", "test.xlsx")
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}
	if _, err = filePart.Write(excelBytes.Bytes()); err != nil {
		t.Fatalf("Failed to copy Excel file data: %v", err)
	}

	// 结束 multipart 编写
	err = writer.Close()
	if err != nil {
		t.Fatalf("Failed to close multipart writer: %v", err)
	}

	// 创建模拟请求
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer test-upload-token")

	// 使用 httptest.NewRecorder() 来模拟响应
	rr := httptest.NewRecorder()

	// 创建服务和处理器
	saver := &MockFreeClassRoomBiz{}
	selectionUploader := &SelectionUploader{
		freeClassRoom: saver,
		uploadToken:   "test-upload-token",
	}

	// 调用 UploadSelection 方法
	selectionUploader.UploadSelection(rr, req)

	t.Log(rr.Body.String())
	// 检查响应状态
	assert.Equal(t, http.StatusOK, rr.Code, "Expected status code 200")
	assert.Equal(t, "2024", saver.year)
	assert.Equal(t, "2", saver.semester)
	assert.NotEmpty(t, saver.ctwPairs)
}

func TestUploadSelectionRequiresAuthorization(t *testing.T) {
	uploader := &SelectionUploader{freeClassRoom: &MockFreeClassRoomBiz{}, uploadToken: "secret"}
	req := httptest.NewRequest(http.MethodPost, "/class_selection/upload", nil)
	rr := httptest.NewRecorder()

	uploader.UploadSelection(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestGetCWTPairsRejectsShortRows(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	const sheet = "data"
	if _, err := f.NewSheet(sheet); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue(sheet, "A2", "星期一第1-2节{1-2周}"); err != nil {
		t.Fatal(err)
	}

	_, err := getCWTPairs(f, map[string]NecessaryIndex{
		sheet: {ClassTimeIdx: 0, ClassWhereIdx: 3},
	})
	if err == nil {
		t.Fatal("expected a short row to be rejected")
	}
}

func TestGetCWTPairsSkipsRowsWithBothConfiguredColumnsMissing(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	const sheet = "data"
	if _, err := f.NewSheet(sheet); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue(sheet, "B2", "星期一第1-2节{1-2周}"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue(sheet, "C2", "N101"); err != nil {
		t.Fatal(err)
	}
	// Keep the row visible to GetRows while leaving both configured columns absent.
	if err := f.SetCellValue(sheet, "A3", "ignored trailing value"); err != nil {
		t.Fatal(err)
	}

	pairs, err := getCWTPairs(f, map[string]NecessaryIndex{
		sheet: {ClassTimeIdx: 1, ClassWhereIdx: 2},
	})
	if err != nil {
		t.Fatalf("expected missing trailing configured cells to be skipped: %v", err)
	}
	if len(pairs) != 1 || pairs[0].Where != "N101" {
		t.Fatalf("unexpected occupancy pairs: %+v", pairs)
	}
}

func TestParseTimeRejectsMalformedValues(t *testing.T) {
	for _, value := range []string{
		"星期一第1-2节",
		"星期一第1-2节{bad}",
		"星期一第0-2节{1-2周}",
		"星期一第1-13节{1-2周}",
	} {
		if _, err := parseTime(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}

package http

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/asynccnu/ccnubox-be/be-class/biz/model"
	"github.com/asynccnu/ccnubox-be/be-class/conf"
	"github.com/xuri/excelize/v2"
)

const (
	maxUploadSize     = 32 << 20
	maxUnzipSize      = 128 << 20
	maxUnzipXMLSize   = 16 << 20
	maxUploadSheets   = 32
	maxWorkbookRows   = 100_000
	maxOccupancyPairs = 1_000_000
)

var (
	academicYearPattern = regexp.MustCompile(`^(\d{4})(?:-(\d{4}))?$`)
	classTimePattern    = regexp.MustCompile(`^星期([一二三四五六日])第(\d{1,2})-(\d{1,2})节\{(.+)\}$`)
	weekPattern         = regexp.MustCompile(`^(\d{1,2})(?:-(\d{1,2}))?周(?:\((单|双)\))?$`)
)

// 必要的列的索引
type NecessaryIndex struct {
	ClassTimeIdx  uint `json:"class_time_idx"`
	ClassWhereIdx uint `json:"class_where_idx"`
}

type UploadReq struct {
	Year     string                    `json:"year"`
	Semester string                    `json:"semester"`
	Sheets   map[string]NecessaryIndex `json:"sheets"` // sheet名，以及每个sheet的上课时间和教学地点的索引[数字],索引从0开始,比如上课时间是第7列，就传6
}

type FreeClassRoomSaver interface {
	SaveFreeClassRoomInfo(ctx context.Context, year, semester string, cwtPairs []model.CTWPair) error
}

// 处理上传选课手册的http服务
type SelectionUploader struct {
	freeClassRoom FreeClassRoomSaver
	uploadToken   string
}

func NewSelectionUploader(freeClassRoom FreeClassRoomSaver, cfg *conf.ServerConf) *SelectionUploader {
	var uploadToken string
	if cfg != nil && cfg.Class != nil {
		uploadToken = strings.TrimSpace(cfg.Class.SelectionUploadToken)
	}
	return &SelectionUploader{
		freeClassRoom: freeClassRoom,
		uploadToken:   uploadToken,
	}
}
func (s *SelectionUploader) UploadSelection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.uploadToken == "" {
		http.Error(w, "Upload endpoint is not configured", http.StatusServiceUnavailable)
		return
	}
	expectedAuthorization := "Bearer " + s.uploadToken
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(expectedAuthorization)) != 1 {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		http.Error(w, "Failed to parse multipart form", http.StatusBadRequest)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	// 解码 JSON
	jsonData := r.FormValue("json_data") // 对应前端字段名
	var req UploadReq
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}
	if err := validateUploadReq(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid upload metadata: %v", err), http.StatusBadRequest)
		return
	}

	// 解析上传的文件
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to get file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	f, err := excelize.OpenReader(file, excelize.Options{
		UnzipSizeLimit:    maxUnzipSize,
		UnzipXMLSizeLimit: maxUnzipXMLSize,
	})
	if err != nil {
		http.Error(w, "Failed to open file", http.StatusBadRequest)
		return
	}
	defer f.Close()

	ctwPairs, err := getCWTPairs(f, req.Sheets)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to handle excel: %v", err), http.StatusBadRequest)
		return
	}

	err = s.freeClassRoom.SaveFreeClassRoomInfo(r.Context(), req.Year, req.Semester, ctwPairs)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to save free classlist room info:%v", err), http.StatusInternalServerError)
		return
	}

	// 设置响应头，内容类型为 JSON
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_, _ = w.Write([]byte(`{"msg":"success"}`))
}

func validateUploadReq(req *UploadReq) error {
	match := academicYearPattern.FindStringSubmatch(strings.TrimSpace(req.Year))
	if match == nil {
		return fmt.Errorf("year must be YYYY or YYYY-YYYY")
	}
	startYear, _ := strconv.Atoi(match[1])
	if match[2] != "" {
		endYear, _ := strconv.Atoi(match[2])
		if endYear != startYear+1 {
			return fmt.Errorf("academic year range is not consecutive")
		}
	}
	if startYear < 2000 || startYear > 3000 {
		return fmt.Errorf("year is out of range")
	}
	req.Year = strconv.Itoa(startYear)
	if req.Semester != "1" && req.Semester != "2" && req.Semester != "3" {
		return fmt.Errorf("semester must be 1, 2, or 3")
	}
	if len(req.Sheets) == 0 || len(req.Sheets) > maxUploadSheets {
		return fmt.Errorf("sheets must contain between 1 and %d entries", maxUploadSheets)
	}
	for sheetName := range req.Sheets {
		if strings.TrimSpace(sheetName) == "" {
			return fmt.Errorf("sheet name cannot be empty")
		}
	}
	return nil
}

func getCWTPairs(f *excelize.File, mp map[string]NecessaryIndex) ([]model.CTWPair, error) {
	type ClassRoomTimeData struct {
		Time  string
		Where string
	}
	var datas []ClassRoomTimeData

	//读取每个sheets的上课时间和上课地点
	for sheetName, necessaryIndex := range mp {
		rows, err := f.GetRows(sheetName)
		if err != nil {
			return nil, err
		}
		for i := 0; i < len(rows); i++ {
			if i == 0 {
				continue
			}
			if len(datas) >= maxWorkbookRows {
				return nil, fmt.Errorf("workbook exceeds %d data rows", maxWorkbookRows)
			}
			var classTime, classWhere string
			if necessaryIndex.ClassTimeIdx < uint(len(rows[i])) {
				classTime = strings.TrimSpace(rows[i][necessaryIndex.ClassTimeIdx])
			}
			if necessaryIndex.ClassWhereIdx < uint(len(rows[i])) {
				classWhere = strings.TrimSpace(rows[i][necessaryIndex.ClassWhereIdx])
			}
			if classTime == "" && classWhere == "" {
				continue
			}
			if classTime == "" || classWhere == "" {
				return nil, fmt.Errorf("sheet %q row %d has incomplete class time or classroom", sheetName, i+1)
			}
			datas = append(datas, ClassRoomTimeData{
				Time:  classTime,
				Where: classWhere,
			})
		}
	}

	var ctwPairs []model.CTWPair

	for _, data := range datas {
		ctimes, err := parseTime(data.Time)
		if err != nil {
			return nil, err
		}
		wheres := strings.Split(data.Where, ";")

		for _, ct := range ctimes {
			for _, where := range wheres {
				where = strings.TrimSpace(where)
				if where == "" {
					return nil, fmt.Errorf("classroom cannot be empty")
				}
				if len(ctwPairs) >= maxOccupancyPairs {
					return nil, fmt.Errorf("workbook expands to more than %d occupancy records", maxOccupancyPairs)
				}
				ctwPairs = append(ctwPairs, model.CTWPair{
					CT:    ct,
					Where: where,
				})
			}
		}
	}
	if len(ctwPairs) == 0 {
		return nil, fmt.Errorf("workbook contains no classroom occupancy data")
	}
	return ctwPairs, nil
}

// 看几种典型的时间格式
// 星期四第3-4节{4-19周}
// 星期一第1-2节{4-18周(双)};星期二第7-8节{4-19周}
// 星期一第5-8节{4-6周(双),7-8周};星期二第5-8节{4-6周(双),7-8周};星期四第1-4节{4-6周(双),7-8周};星期五第1-4节{4-6周(双),7-8周}
// 星期一第9-10节{5-17周(单)};星期二第1-2节{4-19周}
func parseTime(val string) ([]model.CTime, error) {
	var mp = map[string]int{
		"一": 1,
		"二": 2,
		"三": 3,
		"四": 4,
		"五": 5,
		"六": 6,
		"日": 7,
	}

	uniteTimes := strings.Split(val, ";")
	res := make([]model.CTime, 0, len(uniteTimes))
	for _, uniteTime := range uniteTimes {
		uniteTime = strings.TrimSpace(uniteTime)
		match := classTimePattern.FindStringSubmatch(uniteTime)
		if match == nil {
			return nil, fmt.Errorf("invalid class time %q", uniteTime)
		}
		jieStart, _ := strconv.Atoi(match[2])
		jieEnd, _ := strconv.Atoi(match[3])
		if jieStart < 1 || jieEnd > 12 || jieStart > jieEnd {
			return nil, fmt.Errorf("invalid class sections in %q", uniteTime)
		}
		tt := model.CTime{Day: mp[match[1]]}
		for i := jieStart; i <= jieEnd; i++ {
			tt.Sections = append(tt.Sections, i)
		}

		weekStrs := strings.Split(match[4], ",")

		for _, weekStr := range weekStrs {
			weekMatch := weekPattern.FindStringSubmatch(strings.TrimSpace(weekStr))
			if weekMatch == nil {
				return nil, fmt.Errorf("invalid week range %q in %q", weekStr, uniteTime)
			}
			weekStart, _ := strconv.Atoi(weekMatch[1])
			weekEnd := weekStart
			if weekMatch[2] != "" {
				weekEnd, _ = strconv.Atoi(weekMatch[2])
			}
			if weekStart < 1 || weekEnd > 30 || weekStart > weekEnd {
				return nil, fmt.Errorf("week range is out of bounds in %q", uniteTime)
			}
			for i := weekStart; i <= weekEnd; i++ {
				if weekMatch[3] == "" {
					tt.Weeks = append(tt.Weeks, i)
				}
				if weekMatch[3] == "单" {
					if i%2 == 1 {
						tt.Weeks = append(tt.Weeks, i)
					}
				}
				if weekMatch[3] == "双" {
					if i%2 == 0 {
						tt.Weeks = append(tt.Weeks, i)
					}
				}
			}
		}
		if len(tt.Weeks) == 0 {
			return nil, fmt.Errorf("class time %q contains no active weeks", uniteTime)
		}
		res = append(res, tt)
	}
	return res, nil
}

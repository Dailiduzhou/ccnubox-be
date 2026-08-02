package script

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/asynccnu/ccnubox-be/common/pkg/httpx"
)

const (
	freeClassroomQueryURL    = "https://bkzhjw.ccnu.edu.cn/jsxsd/kbxx/jsjy_query2"
	freeClassroomReferer     = "https://bkzhjw.ccnu.edu.cn/jsxsd/kbxx/jsjy_query"
	freeClassroomTimeModelID = "16FD8C2BE55E15F9E0630100007FF6B5"
)

type Data struct {
	ClassRooms []string `json:"class_rooms"`
	PruneStale bool     `json:"prune_stale"`
}

func GetAllClassRooms(year, semester, cookie string) error {
	cli := &http.Client{Timeout: 30 * time.Second}
	classrooms, err := getAllClassRooms(cli, year, semester, cookie)
	if err != nil {
		return err
	}

	f, err := os.Create("repository/classrooms.json")
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	return encoder.Encode(&Data{
		ClassRooms: classrooms,
		PruneStale: false,
	})
}

func getAllClassRooms(cli *http.Client, year, semester, cookie string) ([]string, error) {
	termCode, err := academicTermCode(year, semester)
	if err != nil {
		return nil, err
	}

	form := url.Values{
		"xnxqh":      {termCode},
		"xqbh":       {""},
		"jxqbh":      {""},
		"jxlbh":      {""},
		"jsbh":       {""},
		"jslx":       {""},
		"bjfh":       {"="},
		"rnrs":       {""},
		"yx":         {""},
		"kbjcmsid":   {freeClassroomTimeModelID},
		"selectZc":   {"1"},
		"startdate":  {""},
		"enddate":    {""},
		"selectXq":   {"1,2,3,4,5,6,7"},
		"selectJc":   {"0102,0304,0506,0708,0910,1112"},
		"syjs0601id": {""},
		"typewhere":  {"jszq"},
	}

	req, err := http.NewRequest(http.MethodPost, freeClassroomQueryURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header = http.Header{
		"Accept":           []string{"application/json, text/javascript, */*; q=0.01"},
		"Accept-Language":  []string{"zh-CN,zh;q=0.9"},
		"Content-Type":     []string{"application/x-www-form-urlencoded; charset=UTF-8"},
		"Cookie":           []string{cookie},
		"Origin":           []string{"https://bkzhjw.ccnu.edu.cn"},
		"Referer":          []string{freeClassroomReferer},
		"User-Agent":       []string{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"},
		"X-Requested-With": []string{"XMLHttpRequest"},
	}

	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := httpx.ReadLimited(resp.Body, 1024)
		return nil, fmt.Errorf("classroom upstream returned HTTP %d: %.300s", resp.StatusCode, strings.Join(strings.Fields(string(body)), " "))
	}
	body, err := httpx.ReadResponse(resp)
	if err != nil {
		return nil, err
	}
	return extractClassroomNames(body)
}

func academicTermCode(year, semester string) (string, error) {
	if semester != "1" && semester != "2" && semester != "3" {
		return "", fmt.Errorf("invalid semester %q", semester)
	}
	startYearText := strings.TrimSpace(strings.Split(year, "-")[0])
	startYear, err := strconv.Atoi(startYearText)
	if err != nil || startYear < 2000 || startYear > 3000 {
		return "", fmt.Errorf("invalid academic year %q", year)
	}
	return fmt.Sprintf("%d-%d-%s", startYear, startYear+1, semester), nil
}

func extractClassroomNames(rawJSON []byte) ([]string, error) {
	var response []json.RawMessage
	if err := json.Unmarshal(rawJSON, &response); err != nil {
		return nil, fmt.Errorf("upstream response is not JSON: %w", err)
	}
	if len(response) < 5 {
		return nil, fmt.Errorf("unexpected upstream response length %d", len(response))
	}

	var rows [][]json.RawMessage
	if err := json.Unmarshal(response[4], &rows); err != nil {
		return nil, fmt.Errorf("invalid classroom rows: %w", err)
	}

	classrooms := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for rowIndex, row := range rows {
		if len(row) == 0 {
			return nil, fmt.Errorf("invalid classroom row %d: row is empty", rowIndex)
		}
		var classroom string
		if err := json.Unmarshal(row[0], &classroom); err != nil {
			return nil, fmt.Errorf("invalid classroom row %d name: %w", rowIndex, err)
		}
		classroom = strings.TrimSpace(classroom)
		if classroom == "" {
			return nil, fmt.Errorf("invalid classroom row %d: classroom name is empty", rowIndex)
		}
		if _, ok := seen[classroom]; ok {
			continue
		}
		seen[classroom] = struct{}{}
		classrooms = append(classrooms, classroom)
	}
	if len(classrooms) == 0 {
		return nil, fmt.Errorf("upstream returned no classrooms")
	}
	return classrooms, nil
}

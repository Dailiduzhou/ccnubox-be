package tool

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

func CheckSY(semester, year string) bool {
	var tag1, tag2 bool
	y, err := strconv.Atoi(year)
	currentYear := time.Now().Year()
	if err != nil || y < 2006 || y >= currentYear+2 { // 年份小于2006或者年份大于后年的不予处理
		tag1 = false
	} else {
		tag1 = true
	}
	if semester == "1" || semester == "2" || semester == "3" {
		tag2 = true
	} else {
		tag2 = false
	}
	return tag1 && tag2
}

func ParseWeeks(weeks int64) []int {
	if weeks <= 0 {
		return []int{}
	}
	var weeksList []int
	for i := 1; (1 << (i - 1)) <= weeks; i++ {
		if weeks&(1<<(i-1)) != 0 {
			weeksList = append(weeksList, i)
		}
	}
	return weeksList
}

func FormatWeeks(weeks []int) string {
	if len(weeks) == 0 {
		return ""
	}

	// 对周数集合排序
	sort.Ints(weeks)

	var result strings.Builder
	start := weeks[0]
	end := start
	isSingle := start%2 != 0
	isMixed := false

	// 检查是否是单周、双周还是混合
	for _, week := range weeks {
		if (week%2 == 0) != !isSingle {
			isMixed = true
		}
	}

	// 遍历周数集合，生成格式化字符串
	for i := 1; i < len(weeks); i++ {
		if weeks[i] == end+1 {
			end = weeks[i]
		} else {
			if start == end {
				result.WriteString(strconv.Itoa(start))
			} else {
				result.WriteString(strconv.Itoa(start) + "-" + strconv.Itoa(end))
			}
			result.WriteString(",")
			start = weeks[i]
			end = start
		}
	}

	// 处理最后一段区间
	if start == end {
		result.WriteString(strconv.Itoa(start))
	} else {
		result.WriteString(strconv.Itoa(start) + "-" + strconv.Itoa(end))
	}

	// 添加 "(单)" 或 "(双)" 标识
	if !isMixed {
		if isSingle {
			result.WriteString("周(单)")
		} else {
			result.WriteString("周(双)")
		}
	} else {
		result.WriteString("周")
	}

	return result.String()
}

func ParseClassSections(classWhen string) (int64, error) {
	normalized := strings.NewReplacer(
		"小节", "",
		"节", "",
		" ", "",
		"，", ",",
		"、", ",",
		";", ",",
		"；", ",",
		"~", "-",
		"－", "-",
		"—", "-",
	).Replace(classWhen)
	if normalized == "" {
		return 0, fmt.Errorf("empty class section")
	}

	var sections int64
	for _, part := range strings.Split(normalized, ",") {
		if part == "" {
			return 0, fmt.Errorf("empty class section part")
		}

		start, end, err := parseSectionRange(part)
		if err != nil {
			return 0, err
		}
		for section := start; section <= end; section++ {
			sections |= 1 << (section - 1)
		}
	}
	return sections, nil
}

func parseSectionRange(part string) (int, int, error) {
	rangeParts := strings.Split(part, "-")
	if len(rangeParts) > 2 {
		return 0, 0, fmt.Errorf("invalid class section range: %s", part)
	}

	start, err := strconv.Atoi(rangeParts[0])
	if err != nil || start <= 0 {
		return 0, 0, fmt.Errorf("invalid class section start: %s", part)
	}

	end := start
	if len(rangeParts) == 2 {
		end, err = strconv.Atoi(rangeParts[1])
		if err != nil || end <= 0 {
			return 0, 0, fmt.Errorf("invalid class section end: %s", part)
		}
	}
	if start > end {
		return 0, 0, fmt.Errorf("invalid class section range: %s", part)
	}
	return start, end, nil
}

// ToShanghaiTime 将 time.Time 转换为上海时区的 time.Time
func ToShanghaiTime(t time.Time) time.Time {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	return t.In(loc)
}

// MaskStudentID 脱敏学号：保留前2后2，中间用 **** 隐藏
// 保留前后缀便于日志排查时区分学生，同时隐藏中间部分
func MaskStudentID(value string) string {
	if len(value) <= 4 {
		return "****"
	}
	return value[:2] + "****" + value[len(value)-2:]
}

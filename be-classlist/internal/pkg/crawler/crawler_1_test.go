package crawler

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/asynccnu/ccnubox-be/common/bizpkg/proxy"
)

type MockProxyClient struct{}

func (*MockProxyClient) NewProxyClient(...proxy.Option) *proxy.HttpClient {
	return &proxy.HttpClient{Client: &http.Client{Timeout: 30 * time.Second}}
}

func integrationCredentials(tb testing.TB, cookieEnv string) (string, string) {
	tb.Helper()
	studentID := os.Getenv("CCNU_TEST_STUDENT_ID")
	cookie := os.Getenv(cookieEnv)
	if studentID == "" || cookie == "" {
		tb.Skip("set CCNU_TEST_STUDENT_ID and " + cookieEnv + " to run integration test")
	}
	return studentID, cookie
}

func TestCrawler_GetClassInfosForUndergraduate(t *testing.T) {
	studentID, cookie := integrationCredentials(t, "CCNU_TEST_UNDERGRAD_COOKIE")
	crawler := NewClassCrawler(&MockProxyClient{})
	start := time.Now()
	infos, scs, _, err := crawler.GetClassInfosForUndergraduate(context.Background(), studentID, "2024", "2", cookie)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(fmt.Sprintf("一共耗时%v", time.Since(start)))

	for _, v := range infos {
		t.Log(*v)
	}
	for _, v := range scs {
		t.Log(*v)
	}
	//t.Log(infos, scs)
}

func BenchmarkCrawler_GetClassInfosForUndergraduate(b *testing.B) {
	studentID, cookie := integrationCredentials(b, "CCNU_TEST_UNDERGRAD_COOKIE")
	crawler := NewClassCrawler(&MockProxyClient{})

	ctx := context.Background()

	// 通常第一次调用可以预热缓存等，不纳入统计
	_, _, _, _ = crawler.GetClassInfosForUndergraduate(ctx, studentID, "2024", "2", cookie)

	b.ResetTimer() // 重置计时器，排除预热时间
	for i := 0; i < b.N; i++ {
		_, _, _, err := crawler.GetClassInfosForUndergraduate(ctx, studentID, "2024", "2", cookie)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestCrawler_GetClassInfoForGraduateStudent(t *testing.T) {
	studentID, cookie := integrationCredentials(t, "CCNU_TEST_GRAD_COOKIE")
	crawler := NewClassCrawler(&MockProxyClient{})
	start := time.Now()
	infos, scs, _, err := crawler.GetClassInfoForGraduateStudent(context.Background(), studentID, "2024", "1", cookie)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(fmt.Sprintf("一共耗时%v", time.Since(start)))

	for _, v := range infos {
		t.Log(*v)
	}
	for _, v := range scs {
		t.Log(*v)
	}
}

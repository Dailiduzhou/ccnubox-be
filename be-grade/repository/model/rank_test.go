package model

import (
	"reflect"
	"testing"
)

func TestRankIDTags(t *testing.T) {
	field, ok := reflect.TypeOf(Rank{}).FieldByName("Id")
	if !ok {
		t.Fatal("Rank.Id field not found")
	}
	if got := field.Tag.Get("json"); got != "id" {
		t.Fatalf("Rank.Id json tag = %q, want %q", got, "id")
	}
	if got := field.Tag.Get("gorm"); got != "primaryKey;autoIncrement" {
		t.Fatalf("Rank.Id gorm tag = %q, want primary key and auto increment", got)
	}
}

package services

import (
	"strings"
	"time"
)

func buildCaption(base string, createdTime *time.Time, location *string, labels []string) string {
	pieces := []string{}
	if strings.TrimSpace(base) != "" {
		pieces = append(pieces, base)
	}
	if createdTime != nil {
		pieces = append(pieces, createdTime.Format("2006-01-02"))
	}
	if location != nil {
		pieces = append(pieces, "場所:"+*location)
	}
	if len(labels) > 0 {
		pieces = append(pieces, "ラベル:"+strings.Join(labels, ","))
	}
	if len(pieces) == 0 {
		return "写真の説明"
	}
	return strings.Join(pieces, " ")
}

func truncateText(text string, maxLength int) string {
	if maxLength <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxLength {
		return text
	}
	return string(runes[:maxLength])
}

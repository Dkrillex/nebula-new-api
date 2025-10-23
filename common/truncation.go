package common

import (
	"fmt"
	"strings"
)

// TruncateBase64Content 截断JSON字符串中的base64内容，保留其他信息
// 支持多种格式：
// 1. Gemini 的 inlineData 格式
// 2. 传统的 data:image/ 格式
// 3. 纯 base64 数据
func TruncateBase64Content(content string) string {
	const maxBase64Length = 1000

	// 处理 Gemini 的 inlineData 格式
	content = truncateInlineDataBase64(content, maxBase64Length)

	// 处理传统的 data:image/ 格式
	content = truncateDataImageBase64(content, maxBase64Length)

	// 处理没有前缀的纯 base64 数据
	content = truncateRawBase64Content(content)

	return content
}

// truncateInlineDataBase64 处理 Gemini 的 inlineData 格式中的 base64 数据
// 同时处理 data:video/mp4;base64, 格式的视频数据
func truncateInlineDataBase64(content string, maxLength int) string {
	const inlineDataPattern = `"inlineData":`
	const dataPattern = `"data":`
	const dataVideoPattern = `data:video/`
	const dataImagePattern = `data:image/`

	var result strings.Builder
	startIndex := 0

	for {
		// 查找 inlineData 字段
		inlineDataIndex := strings.Index(content[startIndex:], inlineDataPattern)
		if inlineDataIndex == -1 {
			// 没有更多 inlineData，添加剩余部分
			result.WriteString(content[startIndex:])
			break
		}
		inlineDataIndex += startIndex

		// 添加 inlineData 前的内容
		result.WriteString(content[startIndex:inlineDataIndex])

		// 查找 data 字段
		dataIndex := strings.Index(content[inlineDataIndex:], dataPattern)
		if dataIndex == -1 {
			// 没找到 data 字段，保持原样
			result.WriteString(content[inlineDataIndex:])
			break
		}
		dataIndex += inlineDataIndex

		// 查找 data 值的开始位置（冒号后的引号）
		colonIndex := strings.Index(content[dataIndex:], ":")
		if colonIndex == -1 {
			result.WriteString(content[inlineDataIndex:])
			break
		}
		colonIndex += dataIndex

		// 查找引号开始位置
		quoteStartIndex := strings.Index(content[colonIndex:], "\"")
		if quoteStartIndex == -1 {
			result.WriteString(content[inlineDataIndex:])
			break
		}
		quoteStartIndex += colonIndex + 1

		// 查找引号结束位置
		quoteEndIndex := strings.Index(content[quoteStartIndex:], "\"")
		if quoteEndIndex == -1 {
			result.WriteString(content[inlineDataIndex:])
			break
		}
		quoteEndIndex += quoteStartIndex

		// 获取引号内的内容
		quotedContent := content[quoteStartIndex:quoteEndIndex]

		// 检查是否是 data:video/ 或 data:image/ 格式
		if strings.HasPrefix(quotedContent, dataVideoPattern) || strings.HasPrefix(quotedContent, dataImagePattern) {
			// 处理 data:video/mp4;base64, 或 data:image/ 格式
			truncateDataUrlBase64(content, inlineDataIndex, quoteStartIndex, quoteEndIndex, maxLength, &result)
			startIndex = quoteEndIndex + 1
		} else {
			// 计算 base64 数据长度
			base64DataLength := quoteEndIndex - quoteStartIndex

			// 如果 base64 数据长度超过指定长度，则截断
			if base64DataLength > maxLength {
				// 保留前缀和部分 base64 数据
				result.WriteString(content[inlineDataIndex:quoteStartIndex])
				result.WriteString(content[quoteStartIndex : quoteStartIndex+maxLength])
				result.WriteString("...[base64数据已截断，长度:")
				result.WriteString(fmt.Sprintf("%d", base64DataLength))
				result.WriteString("]\"")
				startIndex = quoteEndIndex + 1
			} else {
				// 短数据保持原样
				result.WriteString(content[inlineDataIndex : quoteEndIndex+1])
				startIndex = quoteEndIndex + 1
			}
		}
	}

	return result.String()
}

// truncateDataUrlBase64 处理 data:video/ 或 data:image/ 格式的 base64 数据
func truncateDataUrlBase64(content string, inlineDataIndex, quoteStartIndex, quoteEndIndex, maxLength int, result *strings.Builder) {
	quotedContent := content[quoteStartIndex:quoteEndIndex]

	// 查找 base64 数据的开始位置
	base64Marker := ";base64,"
	base64StartIndex := strings.Index(quotedContent, base64Marker)

	if base64StartIndex == -1 {
		// 没找到 base64 标记，保持原样
		result.WriteString(content[inlineDataIndex : quoteEndIndex+1])
		return
	}

	base64StartIndex += quoteStartIndex + len(base64Marker)
	base64DataLength := quoteEndIndex - base64StartIndex

	// 如果 base64 数据长度超过指定长度，则截断
	if base64DataLength > maxLength {
		// 保留前缀和部分 base64 数据
		result.WriteString(content[inlineDataIndex:base64StartIndex])
		result.WriteString(content[base64StartIndex : base64StartIndex+maxLength])
		result.WriteString("...[base64数据已截断，长度:")
		result.WriteString(fmt.Sprintf("%d", base64DataLength))
		result.WriteString("]\"")
	} else {
		// 短数据保持原样
		result.WriteString(content[inlineDataIndex : quoteEndIndex+1])
	}
}

// truncateDataImageBase64 处理传统的 data:image/ 和 data:video/ 格式中的 base64 数据
func truncateDataImageBase64(content string, maxLength int) string {
	const base64ImagePrefix = "data:image/"
	const base64VideoPrefix = "data:video/"
	const base64Marker = ";base64,"

	var result strings.Builder
	startIndex := 0

	for {
		// 查找base64前缀（image或video）
		imageIndex := strings.Index(content[startIndex:], base64ImagePrefix)
		videoIndex := strings.Index(content[startIndex:], base64VideoPrefix)

		var base64Index int
		if imageIndex == -1 && videoIndex == -1 {
			break
		} else if imageIndex == -1 {
			base64Index = videoIndex + startIndex
		} else if videoIndex == -1 {
			base64Index = imageIndex + startIndex
		} else {
			// 两个都找到了，选择更早出现的
			if imageIndex < videoIndex {
				base64Index = imageIndex + startIndex
			} else {
				base64Index = videoIndex + startIndex
			}
		}

		// 添加base64前的内容
		result.WriteString(content[startIndex:base64Index])

		// 查找base64标记
		markerIndex := strings.Index(content[base64Index:], base64Marker)
		if markerIndex == -1 {
			// 没找到base64标记，保持原样
			result.WriteString(content[base64Index:])
			break
		}
		markerIndex += base64Index
		base64StartIndex := markerIndex + len(base64Marker)

		// 查找base64数据的结束位置（下一个双引号或字符串末尾）
		base64EndIndex := strings.Index(content[base64StartIndex:], "\"")
		if base64EndIndex == -1 {
			// 没找到结束引号，base64数据一直到字符串末尾
			base64EndIndex = len(content)
		} else {
			base64EndIndex += base64StartIndex
		}

		// 计算base64数据长度
		base64DataLength := base64EndIndex - base64StartIndex

		// 如果base64数据长度超过指定长度，则截断
		if base64DataLength > maxLength {
			// 保留前缀和部分base64数据
			result.WriteString(content[base64Index:base64StartIndex])
			result.WriteString(content[base64StartIndex : base64StartIndex+maxLength])
			result.WriteString("...[base64数据已截断，长度:")
			result.WriteString(fmt.Sprintf("%d", base64DataLength))
			result.WriteString("]")
			// 如果base64数据到字符串末尾，不需要添加引号
			if base64EndIndex < len(content) {
				result.WriteString("\"")
			}
			startIndex = base64EndIndex
		} else {
			// 短数据保持原样
			if base64EndIndex < len(content) {
				result.WriteString(content[base64Index : base64EndIndex+1])
				startIndex = base64EndIndex + 1
			} else {
				result.WriteString(content[base64Index:])
				startIndex = base64EndIndex
			}
		}
	}

	// 添加剩余内容
	result.WriteString(content[startIndex:])
	return result.String()
}

// truncateRawBase64Content 处理没有前缀的纯base64数据
func truncateRawBase64Content(content string) string {
	const maxBase64Length = 1000
	const minBase64Length = 2000 // 只有超过这个长度的才认为是需要截断的base64数据

	var result strings.Builder
	startIndex := 0

	for {
		// 查找可能的base64数据开始位置（以双引号开始的长字符串）
		quoteIndex := strings.Index(content[startIndex:], "\"")
		if quoteIndex == -1 {
			// 没有更多引号，添加剩余部分
			result.WriteString(content[startIndex:])
			break
		}
		quoteIndex += startIndex

		// 添加引号前的内容
		result.WriteString(content[startIndex : quoteIndex+1])

		// 查找下一个引号
		nextQuoteIndex := strings.Index(content[quoteIndex+1:], "\"")
		if nextQuoteIndex == -1 {
			// 没找到结束引号，保持原样
			result.WriteString(content[quoteIndex+1:])
			break
		}
		nextQuoteIndex += quoteIndex + 1

		// 获取引号内的内容
		quotedContent := content[quoteIndex+1 : nextQuoteIndex]

		// 检查是否是base64数据（长度足够且包含base64字符）
		if len(quotedContent) > minBase64Length && isBase64String(quotedContent) {
			// 这是base64数据，需要截断
			if len(quotedContent) > maxBase64Length {
				result.WriteString(quotedContent[:maxBase64Length])
				result.WriteString("...[base64数据已截断，长度:")
				result.WriteString(fmt.Sprintf("%d", len(quotedContent)))
				result.WriteString("]")
			} else {
				result.WriteString(quotedContent)
			}
		} else {
			// 不是base64数据，保持原样
			result.WriteString(quotedContent)
		}

		startIndex = nextQuoteIndex + 1
	}

	return result.String()
}

// isBase64String 检查字符串是否可能是base64数据
func isBase64String(s string) bool {
	if len(s) == 0 {
		return false
	}

	// 检查是否只包含base64字符
	base64Chars := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/="
	base64CharCount := 0

	for _, char := range s {
		if strings.ContainsRune(base64Chars, char) {
			base64CharCount++
		}
	}

	// 如果超过80%的字符是base64字符，且长度足够，则认为是base64
	return float64(base64CharCount)/float64(len(s)) > 0.8
}

package handlers

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/ikermy/Bulk/internal/limits"
)

var (
	ErrFileTooLarge       = errors.New("file too large")
	ErrInvalidFileFormat  = errors.New("invalid file format")
	ErrInvalidContentType = errors.New("invalid content type")
)

// ValidateUpload make assignable for tests: выставляем переменную, чтобы в тестах
// можно было подменять поведение проверки без изменения реализации.
var ValidateUpload = validateUploadImpl

// note: max file size default comes from central limits store (ТЗ §11.2)
const maxFileSize = 10 * 1024 * 1024 // fallback 10MB

var allowedExtensions = map[string]bool{
	".xls":  true,
	".xlsx": true,
	".csv":  true,
}

// Комментарий (соответствие ТЗ §5.1):
// - Валидация разрешает расширения .xlsx, .xls и .csv. Парсинг .xlsx выполняется
//   через библиотеку excelize, CSV через encoding/csv. Для старого бинарного формата
//   .xls парсинг реализован в `pkg/xls/reader.go` с использованием библиотеки
//   github.com/extrame/xls (через временный файл). Таким образом, проект
//   поддерживает все форматы, указанные в ТЗ §5.1.

// ValidateUpload checks size, extension, magic bytes and content-type
//
// Комментарий по безопасности:
//   - Проверка использует header.Size (поставляемый multipart.FileHeader) и runtime-лимит
//     из `limits`. Это быстрый способ отрезать слишком большие загрузки.
//   - Для детектирования типа файла читаются первые 512 байт и используется
//     http.DetectContentType + расширение-специфичные сигнатуры (PK для .xlsx, OLE для .xls).
//   - Эта функция должна вызываться ДО того, как содержимое будет прочитано целиком
//     и передано в парсер/сервис — что и реализовано в обработчике загрузки.
func validateUploadImpl(file multipart.File, header *multipart.FileHeader) error {
	// consult runtime limits
	l := limits.Get()
	if header.Size > int64(l.MaxFileSize) {
		return ErrFileTooLarge
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExtensions[ext] {
		return ErrInvalidFileFormat
	}

	// read up to 512 bytes for sniffing
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return err
	}
	// reset reader to beginning
	if seeker, ok := file.(io.Seeker); ok {
		_, _ = seeker.Seek(0, io.SeekStart)
	}

	sniff := http.DetectContentType(buf[:n])

	// extension-specific checks
	switch ext {
	case ".xlsx":
		// xlsx is a zip archive; check PK signature
		if n < 4 || !(buf[0] == 'P' && buf[1] == 'K' && (buf[2] == 3 || buf[2] == 5 || buf[2] == 7)) {
			// allow also content type detection of zip
			if !strings.Contains(sniff, "zip") && sniff != "application/zip" {
				return ErrInvalidFileFormat
			}
		}
	case ".xls":
		// legacy xls (OLE Compound File) starts with D0 CF 11 E0
		if n < 8 || !(buf[0] == 0xD0 && buf[1] == 0xCF && buf[2] == 0x11 && buf[3] == 0xE0) {
			// sometimes content-type may be application/vnd.ms-excel
			if !strings.Contains(sniff, "ms-excel") && !strings.Contains(sniff, "application/octet-stream") {
				return ErrInvalidFileFormat
			}
		}
	case ".csv":
		// CSV is text-based; accept text/* or application/csv or detect commas/newlines
		if !strings.HasPrefix(sniff, "text/") && !strings.Contains(sniff, "csv") && sniff != "application/octet-stream" {
			// fallback: check for comma/newline in buffer
			s := string(buf[:n])
			if !strings.Contains(s, ",") && !strings.Contains(s, "\n") {
				return ErrInvalidContentType
			}
		}
	default:
		return ErrInvalidFileFormat
	}

	return nil
}

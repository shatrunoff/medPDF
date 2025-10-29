package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

	gofpdf "github.com/phpdave11/gofpdf"
)

const (
	baseFotoDir = "foto"
	basePDFDir  = "pdf"
)

func main() {
	log.SetFlags(0)

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "add":
		runAdd(os.Args[2:])
	case "regen":
		runRegen(os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
	default:
		log.Printf("Неизвестная команда: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("PDFmed — консольное приложение для управления фото анализов и генерации PDF по специализациям.")
	fmt.Println()
	fmt.Println("Использование:")
	fmt.Println("  pdfmed add -p <путь_к_фото> -s <специализация> -d <дата: DD-MM-YYYY> [-n <префикс_имени>]")
	fmt.Println("  pdfmed regen [-s <специализация>]")
	fmt.Println()
	fmt.Println("Команды:")
	fmt.Println("  add    — добавить фото (конвертация в JPG) и перегенерировать PDF")
	fmt.Println("  regen  — перегенерировать PDF (для всех или одной специализации)")
	fmt.Println()
	fmt.Println("Примеры:")
	fmt.Println("  pdfmed add -p /path/to/IMG_001.heic -s \"Эндокринология\" -d 01-01-2024")
	fmt.Println("  pdfmed regen")
	fmt.Println("  pdfmed regen -s \"Эндокринология\"")
}

func runAdd(args []string) {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	var (
		srcPath string
		spec    string
		dateStr string
		name    string
	)
	fs.StringVar(&srcPath, "p", "", "путь к фото (любого поддерживаемого формата)")
	fs.StringVar(&srcPath, "path", "", "путь к фото (любого поддерживаемого формата)")
	fs.StringVar(&spec, "s", "", "специализация (напр. Эндокринология)")
	fs.StringVar(&spec, "spec", "", "специализация (напр. Эндокринология)")
	fs.StringVar(&dateStr, "d", "", "дата анализа в формате DD-MM-YYYY")
	fs.StringVar(&dateStr, "date", "", "дата анализа в формате DD-MM-YYYY")
	fs.StringVar(&name, "n", "", "необязательный префикс имени файла (по умолчанию — специализация)")
	fs.StringVar(&name, "name", "", "необязательный префикс имени файла (по умолчанию — специализация)")
	_ = fs.Parse(args)

	if srcPath == "" || spec == "" || dateStr == "" {
		log.Println("Ошибка: нужно указать -p, -s и -d.")
		fs.Usage()
		os.Exit(1)
	}

	date, formatted, err := ParseDate(dateStr)
	if err != nil {
		log.Fatalf("Неверный формат даты: %v", err)
	}

	if name == "" {
		name = spec
	}

	specSlug := Sanitize(spec)
	nameSlug := Sanitize(name)

	// Создать директории
	fotoDir := filepath.Join(baseFotoDir, specSlug)
	if err := os.MkdirAll(fotoDir, 0o755); err != nil {
		log.Fatalf("Не удалось создать директорию %s: %v", fotoDir, err)
	}
	if err := os.MkdirAll(filepath.Join(basePDFDir, specSlug), 0o755); err != nil {
		log.Fatalf("Не удалось создать директорию pdf/%s: %v", specSlug, err)
	}

	// Конвертировать в JPG и сохранить
	dstBase := fmt.Sprintf("%s_%s.jpg", nameSlug, formatted)
	dstPath := filepath.Join(fotoDir, dstBase)
	dstPath, err = EnsureUniquePath(dstPath)
	if err != nil {
		log.Fatalf("Не удалось подготовить путь назначения: %v", err)
	}

	if err := ConvertToJPG(srcPath, dstPath); err != nil {
		log.Fatalf("Не удалось сконвертировать изображение в JPG: %v", err)
	}

	// Установить mtime файла как указанную дату (удобно для визуального контроля)
	_ = os.Chtimes(dstPath, time.Now(), date)

	log.Printf("Добавлено: %s\n", dstPath)
	if err := GeneratePDFForSpec(specSlug, baseFotoDir, basePDFDir); err != nil {
		log.Fatalf("Ошибка генерации PDF: %v", err)
	}
	log.Println("PDF перегенерирован.")
}

func runRegen(args []string) {
	fs := flag.NewFlagSet("regen", flag.ExitOnError)
	var spec string
	fs.StringVar(&spec, "s", "", "специализация для регенерации (если не указано — для всех)")
	fs.StringVar(&spec, "spec", "", "специализация для регенерации (если не указано — для всех)")
	_ = fs.Parse(args)

	if spec != "" {
		specSlug := Sanitize(spec)
		if err := GeneratePDFForSpec(specSlug, baseFotoDir, basePDFDir); err != nil {
			log.Fatalf("Ошибка генерации PDF для %s: %v", specSlug, err)
		}
		log.Println("Готово.")
		return
	}

	// Для всех специализаций из foto/
	entries, err := ioutil.ReadDir(baseFotoDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Println("Директория foto/ отсутствует. Нечего регенерировать.")
			return
		}
		log.Fatalf("Не удалось прочитать foto/: %v", err)
	}
	if len(entries) == 0 {
		log.Println("В foto/ нет специализаций. Нечего регенерировать.")
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		specSlug := e.Name()
		if err := os.MkdirAll(filepath.Join(basePDFDir, specSlug), 0o755); err != nil {
			log.Fatalf("Не удалось создать директорию pdf/%s: %v", specSlug, err)
		}
		log.Printf("Генерация PDF для: %s...\n", specSlug)
		if err := GeneratePDFForSpec(specSlug, baseFotoDir, basePDFDir); err != nil {
			log.Fatalf("Ошибка генерации PDF для %s: %v", specSlug, err)
		}
	}
	log.Println("Готово.")
}

// ==============
// Helpers merged from internal/files
// ==============

// ParseDate разбирает дату в формате DD-MM-YYYY (также допускает DD.MM.YYYY и DD/MM/YYYY)
// и возвращает время и строку формата DD_MM_YYYY.
func ParseDate(s string) (time.Time, string, error) {
	s = strings.TrimSpace(s)
	layouts := []string{"02-01-2006", "02.01.2006", "02/01/2006"}
	var t time.Time
	var err error
	for _, layout := range layouts {
		t, err = time.Parse(layout, s)
		if err == nil {
			return t, fmt.Sprintf("%02d_%02d_%04d", t.Day(), int(t.Month()), t.Year()), nil
		}
	}
	return time.Time{}, "", fmt.Errorf("ожидался формат DD-MM-YYYY")
}

// Sanitize делает строку безопасной для файловой системы: заменяет пробелы,
// убирает слэши и опасные символы, схлопывает повторы.
func Sanitize(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	s = strings.ReplaceAll(s, ":", "-")
	s = regexp.MustCompile(`[\-_]{2,}`).ReplaceAllString(s, "-")
	return s
}

// EnsureUniquePath добавляет суффиксы _02, _03, ... перед расширением, если путь уже занят.
func EnsureUniquePath(path string) (string, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return path, nil
	}
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	for i := 2; i < 1000; i++ {
		cand := filepath.Join(dir, fmt.Sprintf("%s_%02d%s", name, i, ext))
		if _, err := os.Stat(cand); errors.Is(err, os.ErrNotExist) {
			return cand, nil
		}
	}
	return "", fmt.Errorf("слишком много конфликтов имён для %s", path)
}

// ConvertToJPG конвертирует исходник в JPEG. Сначала пробует встроенные декодеры
// (JPEG/PNG/GIF/WebP/BMP/TIFF), при неудаче — внешние утилиты (ImageMagick/ffmpeg/heif-convert).
func ConvertToJPG(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("не удалось открыть источник: %w", err)
	}
	defer in.Close()

	br := bufio.NewReader(in)
	img, _, err := image.Decode(br)
	if err == nil {
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		out, err := os.Create(dst)
		if err != nil {
			return fmt.Errorf("не удалось создать файл назначения: %w", err)
		}
		defer func() {
			_ = out.Close()
			if err != nil {
				_ = os.Remove(dst)
			}
		}()

		opts := &jpeg.Options{Quality: 85}
		if err := jpeg.Encode(out, img, opts); err != nil {
			return fmt.Errorf("ошибка кодирования JPEG: %w", err)
		}
		return nil
	}

	if err2 := convertToJPGExternal(src, dst); err2 == nil {
		return nil
	}
	return fmt.Errorf("не удалось декодировать изображение встроенными средствами и внешними утилитами: %v", err)
}

// ImageDims быстро возвращает ширину и высоту изображения в пикселях.
func ImageDims(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	var r io.Reader = bufio.NewReader(f)
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

// convertToJPGExternal — попытка конвертации через системные инструменты.
func convertToJPGExternal(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	ext := strings.ToLower(filepath.Ext(src))

	// Видео → первый кадр через ffmpeg
	if isVideoExt(ext) && haveCmd("ffmpeg") {
		if err := runCmd("ffmpeg", "-y", "-i", src, "-frames:v", "1", "-q:v", "2", dst); err == nil {
			return nil
		}
	}

	// ImageMagick (magick или convert)
	if haveCmd("magick") {
		if ext == ".pdf" {
			if err := runCmd("magick", "-density", "300", src+"[0]", "-auto-orient", "-colorspace", "sRGB", "-quality", "85", "-strip", dst); err == nil {
				return nil
			}
		}
		if err := runCmd("magick", src, "-auto-orient", "-colorspace", "sRGB", "-quality", "85", "-strip", dst); err == nil {
			return nil
		}
	}
	if haveCmd("convert") {
		if ext == ".pdf" {
			if err := runCmd("convert", "-density", "300", src+"[0]", "-auto-orient", "-colorspace", "sRGB", "-quality", "85", "-strip", dst); err == nil {
				return nil
			}
		}
		if err := runCmd("convert", src, "-auto-orient", "-colorspace", "sRGB", "-quality", "85", "-strip", dst); err == nil {
			return nil
		}
	}

	// HEIC/HEIF → JPEG через heif-convert
	if (ext == ".heic" || ext == ".heif" || ext == ".heics") && haveCmd("heif-convert") {
		if err := runCmd("heif-convert", src, dst); err == nil {
			return nil
		}
	}
	return fmt.Errorf("нет доступных внешних конверторов (установите ImageMagick и/или ffmpeg)")
}

func haveCmd(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// isVideoExt — определение популярных видеоформатов по расширению.
func isVideoExt(ext string) bool {
	switch ext {
	case ".mp4", ".mov", ".avi", ".mkv", ".mpeg", ".mpg", ".m4v", ".webm":
		return true
	default:
		return false
	}
}

// ==============
// PDF generation merged from internal/pdfgen
// ==============

// fotoItem — элемент источника с датой для сортировки.
type fotoItem struct {
	Path string
	Name string
	Date time.Time
}

var datePattern = regexp.MustCompile(`(\d{2})_(\d{2})_(\d{4})`)

// tryExtractDateFromName — извлекает дату из имени файла формата DD_MM_YYYY.
func tryExtractDateFromName(name string) (time.Time, bool) {
	matches := datePattern.FindAllStringSubmatch(name, -1)
	if len(matches) == 0 {
		return time.Time{}, false
	}
	m := matches[len(matches)-1]
	d, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	y, _ := strconv.Atoi(m[3])
	if d < 1 || d > 31 || mo < 1 || mo > 12 || y < 1 {
		return time.Time{}, false
	}
	t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.Local)
	return t, true
}

// collectJPGsSorted — собирает JPG файлы и сортирует их по дате.
func collectJPGsSorted(dir string) ([]fotoItem, error) {
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var items []fotoItem
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".jpg" && ext != ".jpeg" {
			continue
		}
		p := filepath.Join(dir, name)
		var ft time.Time
		if t, ok := tryExtractDateFromName(name); ok {
			ft = t
		} else {
			ft = e.ModTime()
		}
		items = append(items, fotoItem{Path: p, Name: name, Date: ft})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Date.Equal(items[j].Date) {
			return items[i].Name < items[j].Name
		}
		return items[i].Date.Before(items[j].Date)
	})
	return items, nil
}

// GeneratePDFForSpec — создаёт PDF из JPG по указанной специализации.
func GeneratePDFForSpec(specSlug string, baseFotoDir, basePDFDir string) error {
	srcDir := filepath.Join(baseFotoDir, specSlug)
	if _, err := os.Stat(srcDir); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("директория не найдена: %s", srcDir)
	}

	items, err := collectJPGsSorted(srcDir)
	if err != nil {
		return fmt.Errorf("не удалось собрать изображения: %w", err)
	}
	if len(items) == 0 {
		log.Printf("Предупреждение: в %s нет JPG изображений для генерации PDF\n", srcDir)
	}

	// Настройка PDF: A4, мм, портрет
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetCompression(true)
	pdf.SetTitle(specSlug, false)

	// Размеры страницы A4 в мм
	pageW, pageH := 210.0, 297.0
	margin := 10.0
	maxW := pageW - 2*margin
	maxH := pageH - 2*margin

	for _, it := range items {
		wpx, hpx, err := ImageDims(it.Path)
		if err != nil {
			log.Printf("Пропуск %s: не удалось прочитать размеры: %v\n", it.Name, err)
			continue
		}
		// Масштабируем по ограничивающей стороне внутри полей
		scaleW := maxW / float64(wpx)
		scaleH := maxH / float64(hpx)
		scale := scaleW
		if scaleH < scaleW {
			scale = scaleH
		}
		wmm := float64(wpx) * scale
		hmm := float64(hpx) * scale
		x := (pageW - wmm) / 2.0
		y := (pageH - hmm) / 2.0

		pdf.AddPage()
		opt := gofpdf.ImageOptions{ImageType: "JPG", ReadDpi: true}
		pdf.ImageOptions(it.Path, x, y, wmm, hmm, false, opt, 0, "")
	}

	outDir := filepath.Join(basePDFDir, specSlug)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	outPath := filepath.Join(outDir, fmt.Sprintf("%s.pdf", specSlug))
	if err := pdf.OutputFileAndClose(outPath); err != nil {
		return fmt.Errorf("не удалось сохранить PDF: %w", err)
	}
	log.Printf("PDF создан: %s\n", outPath)
	return nil
}

package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"image"
	_ "image/gif"
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
	fmt.Println("  pdfmed add -p <путь_к_фото_или_pdf> -s <специализация> -d <дата: DD-MM-YYYY> [-n <префикс_имени>]")
	fmt.Println("  pdfmed regen [-s <специализация>]")
	fmt.Println()
	fmt.Println("Команды:")
	fmt.Println("  add    — добавить фото или PDF (конвертация в JPG) и перегенерировать PDF")
	fmt.Println("  regen  — перегенерировать PDF (для всех или одной специализации)")
	fmt.Println()
	fmt.Println("Примеры:")
	fmt.Println("  pdfmed add -p /path/to/IMG_001.heic -s \"Эндокринология\" -d 01-01-2024")
	fmt.Println("  pdfmed add -p /path/to/report.pdf -s \"Гастроэнтерология\" -d 15-02-2024")
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
	fs.StringVar(&srcPath, "p", "", "путь к фото или PDF")
	fs.StringVar(&srcPath, "path", "", "путь к фото или PDF")
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

	fotoDir := filepath.Join(baseFotoDir, specSlug)
	if err := os.MkdirAll(fotoDir, 0o755); err != nil {
		log.Fatalf("Не удалось создать директорию %s: %v", fotoDir, err)
	}
	if err := os.MkdirAll(filepath.Join(basePDFDir, specSlug), 0o755); err != nil {
		log.Fatalf("Не удалось создать директорию pdf/%s: %v", specSlug, err)
	}

	ext := strings.ToLower(filepath.Ext(srcPath))
	if ext == ".pdf" {
		log.Println("Обнаружен PDF, выполняется конвертация страниц в JPG...")
		pages, err := ConvertPDFToJPGs(srcPath, fotoDir, fmt.Sprintf("%s_%s", nameSlug, formatted))
		if err != nil {
			log.Fatalf("Ошибка конвертации PDF: %v", err)
		}
		for _, p := range pages {
			_ = os.Chtimes(p, time.Now(), date)
			log.Printf("Добавлена страница: %s\n", p)
		}
	} else {
		dstBase := fmt.Sprintf("%s_%s.jpg", nameSlug, formatted)
		dstPath := filepath.Join(fotoDir, dstBase)
		dstPath, err = EnsureUniquePath(dstPath)
		if err != nil {
			log.Fatalf("Не удалось подготовить путь назначения: %v", err)
		}
		if err := ConvertToJPG(srcPath, dstPath); err != nil {
			log.Fatalf("Не удалось сконвертировать изображение в JPG: %v", err)
		}
		_ = os.Chtimes(dstPath, time.Now(), date)
		log.Printf("Добавлено: %s\n", dstPath)
	}

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

// ======== Helpers ========

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

func Sanitize(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	s = strings.ReplaceAll(s, ":", "-")
	s = regexp.MustCompile(`[\-_]{2,}`).ReplaceAllString(s, "-")
	return s
}

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

// ======== Конвертация ========

func ConvertPDFToJPGs(srcPDF, dstDir, baseName string) ([]string, error) {
	if !haveCmd("magick") && !haveCmd("convert") {
		return nil, fmt.Errorf("не найден ImageMagick (magick/convert)")
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return nil, err
	}

	pattern := filepath.Join(dstDir, baseName+"_page_%03d.jpg")
	var cmd *exec.Cmd
	if haveCmd("magick") {
		cmd = exec.Command("magick", "-density", "300", srcPDF, "-quality", "85",
			"-auto-orient", "-colorspace", "sRGB", "-strip", pattern)
	} else {
		cmd = exec.Command("convert", "-density", "300", srcPDF, "-quality", "85",
			"-auto-orient", "-colorspace", "sRGB", "-strip", pattern)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ошибка конвертации PDF → JPG: %w", err)
	}

	files, err := filepath.Glob(filepath.Join(dstDir, baseName+"_page_*.jpg"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("не удалось найти JPG-страницы после конвертации")
	}
	return files, nil
}

func ConvertToJPG(src, dst string) error {
	if !haveCmd("magick") && !haveCmd("convert") {
		return fmt.Errorf("не найден ImageMagick (magick/convert)")
	}

	var cmd *exec.Cmd
	if haveCmd("magick") {
		cmd = exec.Command("magick", src, "-auto-orient", "-strip", "-quality", "85", "-colorspace", "sRGB", dst)
	} else {
		cmd = exec.Command("convert", src, "-auto-orient", "-strip", "-quality", "85", "-colorspace", "sRGB", dst)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ошибка конвертации %s → JPG: %w", src, err)
	}
	return nil
}

func convertToJPGExternal(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	ext := strings.ToLower(filepath.Ext(src))
	if isVideoExt(ext) && haveCmd("ffmpeg") {
		if err := runCmd("ffmpeg", "-y", "-i", src, "-frames:v", "1", "-q:v", "2", dst); err == nil {
			return nil
		}
	}
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

func isVideoExt(ext string) bool {
	switch ext {
	case ".mp4", ".mov", ".avi", ".mkv", ".mpeg", ".mpg", ".m4v", ".webm":
		return true
	default:
		return false
	}
}

// ======== PDF Generation ========

type fotoItem struct {
	Path string
	Name string
	Date time.Time
}

var datePattern = regexp.MustCompile(`(\d{2})_(\d{2})_(\d{4})`)

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
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetCompression(true)
	pdf.SetTitle(specSlug, false)
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

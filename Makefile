# Параметры
APP_DIR := PDFmed
BIN := pdfmed
GO ?= go
PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
DESTDIR ?=
SUDO ?=

.PHONY: help build clean new regen tidy fmt install uninstall install-tools

help:
	@echo "Доступные команды:"
	@echo "  make build                — собрать бинарник $(BIN)"
	@echo "  make install             — установить бинарник в $(DESTDIR)$(BINDIR)"
	@echo "  make uninstall           — удалить установленный бинарник"
	@echo "  make install-tools       — установить ImageMagick, ffmpeg, libheif (если возможно)"
	@echo "  make new FILE=... SPEC=... DATE=DD-MM-YYYY [NAME=...]"
	@echo "                           — добавить фото и пересобрать PDF"
	@echo "  make regen [SPEC=...]    — пересобрать PDF для всех или одной специализации"
	@echo "  make tidy                — go mod tidy внутри $(APP_DIR)"
	@echo "  make fmt                 — go fmt исходники"
	@echo "  make clean               — удалить бинарник $(BIN)"


	

build:
	@cd $(APP_DIR) && $(GO) mod tidy
	@cd $(APP_DIR) && $(GO) mod vendor
	@cd $(APP_DIR) && $(GO) build -o ../$(BIN) .

install: build
	@echo "Установка $(BIN) в $(DESTDIR)$(BINDIR)"
	@install -d "$(DESTDIR)$(BINDIR)"
	@$(SUDO) install -m 0755 "$(BIN)" "$(DESTDIR)$(BINDIR)/$(BIN)"
	@echo "Готово: $(DESTDIR)$(BINDIR)/$(BIN)"

uninstall:
	@echo "Удаление $(DESTDIR)$(BINDIR)/$(BIN)"
	@$(SUDO) rm -f "$(DESTDIR)$(BINDIR)/$(BIN)"

tidy:
	@cd $(APP_DIR) && $(GO) mod tidy

fmt:
	@cd $(APP_DIR) && $(GO) fmt ./...

new: $(BIN)
	@test -n "$(FILE)" || (echo "ERROR: укажите FILE=/путь/к/фото" && exit 1)
	@test -n "$(SPEC)" || (echo "ERROR: укажите SPEC=специализация" && exit 1)
	@test -n "$(DATE)" || (echo "ERROR: укажите DATE=DD-MM-YYYY" && exit 1)
	@./$(BIN) add -p "$(FILE)" -s "$(SPEC)" -d "$(DATE)" -n "$(NAME)"

regen: $(BIN)
	@if [ -n "$(SPEC)" ]; then \
		./$(BIN) regen -s "$(SPEC)"; \
	else \
		./$(BIN) regen; \
	fi

install-tools:
	@bash -eu -c '\
	if command -v apt-get >/dev/null 2>&1; then \
		$(SUDO) apt-get update; \
		$(SUDO) apt-get install -y imagemagick ffmpeg || true; \
		$(SUDO) apt-get install -y libheif-examples libheif-tools || true; \
		echo "Пакеты установлены (apt)."; \
	elif command -v dnf >/dev/null 2>&1; then \
		$(SUDO) dnf install -y ImageMagick ffmpeg libheif libheif-tools || true; \
		echo "Пакеты установлены (dnf)."; \
	elif command -v yum >/dev/null 2>&1; then \
		$(SUDO) yum install -y ImageMagick ffmpeg libheif libheif-tools || true; \
		echo "Пакеты установлены (yum)."; \
	elif command -v pacman >/dev/null 2>&1; then \
		$(SUDO) pacman -Syu --noconfirm imagemagick ffmpeg libheif || true; \
		echo "Пакеты установлены (pacman)."; \
	elif command -v zypper >/dev/null 2>&1; then \
		$(SUDO) zypper --non-interactive install ImageMagick ffmpeg libheif-tools || true; \
		echo "Пакеты установлены (zypper)."; \
	elif command -v apk >/dev/null 2>&1; then \
		$(SUDO) apk add --no-cache imagemagick ffmpeg libheif-tools || true; \
		echo "Пакеты установлены (apk)."; \
	elif command -v brew >/dev/null 2>&1; then \
		brew update; \
		brew install imagemagick ffmpeg libheif || true; \
		echo "Пакеты установлены (brew)."; \
	elif command -v choco >/dev/null 2>&1; then \
		echo "Обнаружен Windows (choco). Запустите от администратора:"; \
		echo "  choco install -y imagemagick ffmpeg"; \
		echo "  choco install -y libheif || echo 'установите libheif вручную'"; \
	elif command -v winget >/dev/null 2>&1; then \
		echo "Обнаружен Windows (winget). Выполните:"; \
		echo "  winget install --id=ImageMagick.ImageMagick -e"; \
		echo "  winget install --id=Gyan.FFmpeg -e"; \
		echo "  winget install --id=strukturag.Libheif -e || echo 'установите libheif вручную'"; \
	else \
		echo "Не удалось определить пакетный менеджер. Установите вручную: ImageMagick, ffmpeg, libheif/heif-convert."; \
	fi'

clean:
	@rm -f $(BIN)

test-add:
	./PDFmed/medPDF add -p 000070860041.jpg -s "Эндокринология" -d 01-02-2001
	./PDFmed/medPDF add -p 000070860042.jpg -s "Эндокринология" -d 01-02-2000
	./PDFmed/medPDF add -p IMG_2251.HEIC -s "Эндокринология" -d 01-03-2001
	./PDFmed/medPDF add -p IMG_2252.HEIC -s "Эндокринология" -d 01-03-2001
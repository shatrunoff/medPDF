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
	brew update
	brew install imagemagick ffmpeg libheif ghostscript

clear:
	rm -rf foto pdf

add:
	./PDFmed/medPDF add -p raw_foto/IMG_2361.jpeg -s "Дерматолог" -d 18-08-2025
	./PDFmed/medPDF add -p raw_foto/IMG_2475.jpeg -s "Травматолог" -d 01-08-2025
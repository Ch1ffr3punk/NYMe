package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image/color"
	"math/big"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type greenThemeWrapper struct {
	base fyne.Theme
}

func (g *greenThemeWrapper) Font(s fyne.TextStyle) fyne.Resource {
	if s.Bold && !s.Italic && !s.Monospace {
		if resourceLabGrotesqueBoldTtf != nil {
			return resourceLabGrotesqueBoldTtf
		}
	}
	return theme.DefaultTheme().Font(s)
}

func (g *greenThemeWrapper) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 20, G: 231, B: 111, A: 255}
	case theme.ColorNameForegroundOnPrimary:
		return color.Black
	case theme.ColorNameHyperlink:
		return color.NRGBA{R: 20, G: 231, B: 111, A: 255}
	default:
		return g.base.Color(name, variant)
	}
}

func (g *greenThemeWrapper) Icon(name fyne.ThemeIconName) fyne.Resource {
	return g.base.Icon(name)
}

func (g *greenThemeWrapper) Size(name fyne.ThemeSizeName) float32 {
	return g.base.Size(name)
}

type FormData struct {
	NYMe   string
	Seller string
	Buyer  string
	Tx     string
	Code   string
	Nym    string
}

var (
	generatedIDs   = make(map[string]bool)
	generatedCodes = make(map[string]bool)
	idMutex        = sync.RWMutex{}
	codeMutex      = sync.RWMutex{}
)

type NYMeApp struct {
	app            fyne.App
	window         fyne.Window
	isDarkTheme    bool
	themeSwitch    *widget.Button
	infoBtn        *widget.Button
	resetBtn       *widget.Button
	saveBtn        *widget.Button
	nymEntry       *widget.Entry
	codeEntry      *widget.Entry
	sellerEntry    *widget.Entry
	buyerEntry     *widget.Entry
	txEntry        *widget.Entry
	nymAmountEntry *widget.Entry
	sellerStatus   *widget.Label
	buyerStatus    *widget.Label
	txStatus       *widget.Label
	nymStatus      *widget.Label
}

func main() {
	myApp := app.New()
	myApp.Settings().SetTheme(&greenThemeWrapper{
		base: theme.DarkTheme(),
	})

	window := myApp.NewWindow("NYMe Transaction Tool")
	window.Resize(fyne.NewSize(640, 600))
	window.CenterOnScreen()

	nymApp := &NYMeApp{
		app:         myApp,
		window:      window,
		isDarkTheme: true,
	}

	nymApp.setupUI()
	window.ShowAndRun()
}

func validateNymAmount(s string) error {
	if s == "" {
		return nil
	}
	cleaned := strings.TrimSpace(s)
	if strings.Contains(cleaned, ",") {
		return fmt.Errorf("invalid format: use '.' for decimals, no commas allowed (e.g. 36050.984145)")
	}
	matched, _ := regexp.MatchString(`^\d+(?:\.\d{1,6})?$`, cleaned)
	if !matched {
		return fmt.Errorf("invalid format: use numbers like 36050 or 36050.984145")
	}
	value, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return fmt.Errorf("invalid number format")
	}
	if value <= 0 {
		return fmt.Errorf("amount must be greater than 0")
	}
	if value > 1000000 {
		return fmt.Errorf("amount too high (max 1,000,000 Nym)")
	}
	return nil
}

func normalizeNymInput(s string) string {
	if s == "" {
		return ""
	}
	cleaned := strings.TrimSpace(s)
	if !strings.Contains(cleaned, ".") {
		return cleaned + ".000000"
	}
	parts := strings.Split(cleaned, ".")
	if len(parts) != 2 {
		return cleaned
	}
	decimals := parts[1]
	if len(decimals) < 6 {
		decimals += strings.Repeat("0", 6-len(decimals))
	} else if len(decimals) > 6 {
		value, err := strconv.ParseFloat(cleaned, 64)
		if err == nil {
			return fmt.Sprintf("%.6f", value)
		}
		decimals = decimals[:6]
	}
	return parts[0] + "." + decimals
}

func (c *NYMeApp) setupUI() {
	c.nymEntry = widget.NewEntry()
	c.nymEntry.SetPlaceHolder("Auto-generated unique ID (32 chars hex)")
	c.nymEntry.Text = generateUniqueNYMe()

	c.codeEntry = widget.NewEntry()
	c.codeEntry.SetPlaceHolder("Auto-generated 8-digit code")
	c.codeEntry.Text = generateUniqueCode()

	c.sellerEntry = widget.NewEntry()
	c.sellerEntry.SetPlaceHolder("n1...")
	c.sellerEntry.Validator = func(s string) error {
		if s == "" {
			return nil
		}
		if !strings.HasPrefix(s, "n1") {
			return fmt.Errorf("must start with 'n1'")
		}
		if len(s) < 44 || len(s) > 54 {
			return fmt.Errorf("address must be 44-54 characters long")
		}
		return nil
	}

	c.buyerEntry = widget.NewEntry()
	c.buyerEntry.SetPlaceHolder("n1...")
        c.buyerEntry.Validator = func(s string) error {
		if s == "" {
			return nil
		}
		if !strings.HasPrefix(s, "n1") {
			return fmt.Errorf("must start with 'n1'")
		}
		if len(s) < 44 || len(s) > 54 {
			return fmt.Errorf("address must be 44-54 characters long")
		}
		return nil
	}

	c.txEntry = widget.NewEntry()
	c.txEntry.SetPlaceHolder("")
	c.txEntry.Validator = func(s string) error {
		if s == "" {
			return nil
		}
		if matched, _ := regexp.MatchString("^[A-F0-9]{64}$", s); !matched {
			return fmt.Errorf("must be 64 hex characters (0-9, A-F)")
		}
		return nil
	}

	c.nymAmountEntry = widget.NewEntry()
	c.nymAmountEntry.SetPlaceHolder("")
	c.nymAmountEntry.Validator = validateNymAmount

	c.sellerStatus = widget.NewLabel("")
	c.sellerStatus.Hide()
	c.buyerStatus = widget.NewLabel("")
	c.buyerStatus.Hide()
	c.txStatus = widget.NewLabel("")
	c.txStatus.Hide()
	c.nymStatus = widget.NewLabel("")
	c.nymStatus.Hide()

	c.sellerEntry.OnChanged = func(text string) {
		if text == "" {
			c.sellerStatus.Hide()
		} else if err := c.sellerEntry.Validate(); err == nil {
			c.sellerStatus.SetText("valid ✓")
			c.sellerStatus.Importance = widget.SuccessImportance
			c.sellerStatus.Show()
		} else {
			c.sellerStatus.SetText(err.Error())
			c.sellerStatus.Importance = widget.WarningImportance
			c.sellerStatus.Show()
		}
	}

	c.buyerEntry.OnChanged = func(text string) {
		if text == "" {
			c.buyerStatus.Hide()
		} else if err := c.buyerEntry.Validate(); err == nil {
			c.buyerStatus.SetText("valid ✓")
			c.buyerStatus.Importance = widget.SuccessImportance
			c.buyerStatus.Show()
		} else {
			c.buyerStatus.SetText(err.Error())
			c.buyerStatus.Importance = widget.WarningImportance
			c.buyerStatus.Show()
		}
	}

	c.txEntry.OnChanged = func(text string) {
		if text == "" {
			c.txStatus.Hide()
		} else if err := c.txEntry.Validate(); err == nil {
			c.txStatus.SetText("valid ✓")
			c.txStatus.Importance = widget.SuccessImportance
			c.txStatus.Show()
		} else {
			c.txStatus.SetText(err.Error())
			c.txStatus.Importance = widget.WarningImportance
			c.txStatus.Show()
		}
	}

	c.nymAmountEntry.OnChanged = func(text string) {
		if text == "" {
			c.nymStatus.Hide()
		} else if err := c.nymAmountEntry.Validate(); err == nil {
			normalized := normalizeNymInput(text)
			if normalized != text {
				c.nymStatus.SetText(fmt.Sprintf("valid ✓ (normalized: %s)", normalized))
			} else {
				c.nymStatus.SetText("valid ✓")
			}
			c.nymStatus.Importance = widget.SuccessImportance
			c.nymStatus.Show()
		} else {
			c.nymStatus.SetText(err.Error())
			c.nymStatus.Importance = widget.WarningImportance
			c.nymStatus.Show()
		}
	}

	generateNymBtn := widget.NewButton("Generate New", func() {
		newID := generateUniqueNYMe()
		c.nymEntry.SetText(newID)
	})
	generateNymBtn.Importance = widget.HighImportance

	generateCodeBtn := widget.NewButton("Generate Code", func() {
		newCode := generateUniqueCode()
		c.codeEntry.SetText(newCode)
	})
	generateCodeBtn.Importance = widget.HighImportance

	c.resetBtn = widget.NewButtonWithIcon("Reset", theme.CancelIcon(), func() {
		c.nymEntry.SetText(generateUniqueNYMe())
		c.codeEntry.SetText(generateUniqueCode())
		c.sellerEntry.SetText("")
		c.buyerEntry.SetText("")
		c.txEntry.SetText("")
		c.nymAmountEntry.SetText("")
		c.sellerStatus.Hide()
		c.buyerStatus.Hide()
		c.txStatus.Hide()
		c.nymStatus.Hide()
	})
	c.resetBtn.Importance = widget.MediumImportance

	c.saveBtn = widget.NewButtonWithIcon("Save", theme.ConfirmIcon(), func() {
		nymValue := normalizeNymInput(c.nymAmountEntry.Text)

		data := FormData{
			NYMe:   c.nymEntry.Text,
			Seller: c.sellerEntry.Text,
			Buyer:  c.buyerEntry.Text,
			Tx:     c.txEntry.Text,
			Code:   c.codeEntry.Text,
			Nym:    nymValue,
		}

		if data.NYMe == "" {
			dialog.ShowError(fmt.Errorf("please generate NYMe ID first"), c.window)
			return
		}

		if data.Code == "" {
			dialog.ShowError(fmt.Errorf("please generate authorization code first"), c.window)
			return
		}

		saveToFile(data, c.window)
	})
	c.saveBtn.Importance = widget.HighImportance

	nymContainer := container.NewVBox(
		widget.NewLabel("NYMe (32 hex chars)"),
		c.nymEntry,
		generateNymBtn,
	)

	codeContainer := container.NewVBox(
		widget.NewLabel("Auth Code (8 digits)"),
		c.codeEntry,
		generateCodeBtn,
	)

	topContainer := container.NewGridWithColumns(2,
		nymContainer,
		codeContainer,
	)

	optionalFields := container.NewVBox(
		widget.NewSeparator(),
		container.NewVBox(
			widget.NewLabel("Seller"),
			c.sellerEntry,
			c.sellerStatus,
		),
		container.NewVBox(
			widget.NewLabel("Buyer"),
			c.buyerEntry,
			c.buyerStatus,
		),
		container.NewVBox(
			widget.NewLabel("NYM"),
			c.nymAmountEntry,
			c.nymStatus,
		),
		container.NewVBox(
			widget.NewLabel("Tx Hash"),
			c.txEntry,
			c.txStatus,
		),
	)

	buttonContainer := container.NewHBox(
		layout.NewSpacer(),
		c.resetBtn,
		c.saveBtn,
		layout.NewSpacer(),
	)

	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(0, 15))

	c.themeSwitch = widget.NewButton("☀️", c.toggleTheme)
	c.themeSwitch.Importance = widget.LowImportance

	c.infoBtn = widget.NewButtonWithIcon("", theme.InfoIcon(), c.showInfoPopup)
	c.infoBtn.Importance = widget.LowImportance

	globalTopBar := container.NewHBox(
		c.infoBtn,
		layout.NewSpacer(),
		c.themeSwitch,
	)

	headerContent := container.NewBorder(
		globalTopBar,
		nil,
		nil,
		nil,
		container.NewVBox(
			widget.NewLabelWithStyle("NYMe Transaction Tool", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			widget.NewSeparator(),
			topContainer,
			widget.NewSeparator(),
		),
	)

	bottomContent := container.NewVBox(
		widget.NewSeparator(),
		buttonContainer,
		spacer,
	)

	content := container.NewBorder(
		headerContent,
		bottomContent,
		nil,
		nil,
		container.NewScroll(optionalFields),
	)

	c.window.SetContent(content)
}

func (c *NYMeApp) toggleTheme() {
	c.isDarkTheme = !c.isDarkTheme

	if c.isDarkTheme {
		c.themeSwitch.SetText("☀️")
	} else {
		c.themeSwitch.SetText("🌙")
	}

	var baseTheme fyne.Theme
	if c.isDarkTheme {
		baseTheme = theme.DarkTheme()
	} else {
		baseTheme = theme.LightTheme()
	}

	greenTheme := &greenThemeWrapper{
		base: baseTheme,
	}
	c.app.Settings().SetTheme(greenTheme)
	c.window.Content().Refresh()
}

func (c *NYMeApp) showInfoPopup() {
	projURL, _ := url.Parse("https://github.com/Ch1ffr3punk/NYMe")
	projectLink := widget.NewHyperlink("An Open Source project", projURL)
	okButton := widget.NewButton("OK", func() {
		overlays := c.window.Canvas().Overlays()
		if overlays.Top() != nil {
			overlays.Remove(overlays.Top())
		}
	})
	okButton.Importance = widget.HighImportance

	content := container.NewVBox(
		widget.NewLabelWithStyle("NYMe Transaction Tool v0.1.0", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		container.NewHBox(layout.NewSpacer(), projectLink, layout.NewSpacer()),
		widget.NewLabelWithStyle("released under the Apache 2.0 license", fyne.TextAlignCenter, fyne.TextStyle{}),
		widget.NewLabelWithStyle("2026 Ch1ffr3punk", fyne.TextAlignCenter, fyne.TextStyle{}),
		container.NewHBox(layout.NewSpacer(), okButton, layout.NewSpacer()),
	)
	dialog.ShowCustomWithoutButtons("", content, c.window)
}

func generateUniqueNYMe() string {
	idMutex.Lock()
	defer idMutex.Unlock()

	for {
		bytes := make([]byte, 16)
		_, err := rand.Read(bytes)
		if err != nil {
			return fmt.Sprintf("%d_%s", time.Now().UnixNano(), randomString(16))
		}

		id := hex.EncodeToString(bytes)

		if !generatedIDs[id] {
			generatedIDs[id] = true
			return id
		}
	}
}

func generateUniqueCode() string {
	codeMutex.Lock()
	defer codeMutex.Unlock()

	for {
		max := big.NewInt(100000000)
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return fmt.Sprintf("%08d", time.Now().UnixNano()%100000000)
		}

		code := fmt.Sprintf("%08d", n.Int64())

		if !generatedCodes[code] {
			generatedCodes[code] = true
			return code
		}
	}
}

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		randomBytes := make([]byte, 1)
		rand.Read(randomBytes)
		b[i] = charset[randomBytes[0]%byte(len(charset))]
	}
	return string(b)
}

func saveToFile(data FormData, win fyne.Window) {
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.txt", data.NYMe[:16], timestamp)

	content := strings.Builder{}
	content.WriteString(fmt.Sprintf("NYMe: %s\n", data.NYMe))
	content.WriteString(fmt.Sprintf("Code: %s\n", data.Code))

	if data.Nym != "" {
		content.WriteString(fmt.Sprintf("Nym: %s\n", data.Nym))
	}

	if data.Seller != "" {
		content.WriteString(fmt.Sprintf("Seller: %s\n", data.Seller))
	}
	if data.Buyer != "" {
		content.WriteString(fmt.Sprintf("Buyer: %s\n", data.Buyer))
	}
	if data.Tx != "" {
		content.WriteString(fmt.Sprintf("Tx: %s\n", data.Tx))
	}

	err := os.WriteFile(filename, []byte(content.String()), 0644)
	if err != nil {
		dialog.ShowError(fmt.Errorf("save error: %v", err), win)
		return
	}

	dialog.ShowInformation("Success", fmt.Sprintf("Saved to:\n%s", filename), win)
}

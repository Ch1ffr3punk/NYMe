package main

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"image"
	"image/color"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/awnumar/memguard"
	"github.com/c0mm4nd/go-ripemd"
	"github.com/go-piv/piv-go/v2/piv"
	"github.com/martinlindhe/gogost/gost34112012256"
	"github.com/tjfoc/gmsm/sm3"
)

// Green Theme Wrapper
type greenThemeWrapper struct {
	base fyne.Theme
}

func (g *greenThemeWrapper) Font(s fyne.TextStyle) fyne.Resource {
	return g.base.Font(s)
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

type GUI struct {
	app           fyne.App
	window        fyne.Window
	themeToggle   *widget.Button
	infoBtn       *widget.Button
	hashSelected  string
	hashButtons   map[string]*widget.Button
	pinEntry      *widget.Entry
	statusLabel   *widget.Label
	filenameLabel *widget.Label
	filesizeLabel *widget.Label
	sigDisplay    *widget.Label
	progressBar   *widget.ProgressBar
	progressLabel *widget.Label
	currentTheme  string
	currentFile   string
	signaturePath string
	fileSelected  bool
}

func main() {
	defer memguard.Purge()
	gui := &GUI{
		app:          app.NewWithID("oc2mx.net.nyme"),
		currentTheme: "dark",
		hashSelected: "RIPEMD-256",
		hashButtons:  make(map[string]*widget.Button),
	}
	
	// Set green theme
	gui.app.Settings().SetTheme(&greenThemeWrapper{
		base: theme.DarkTheme(),
	})
	
	gui.window = gui.app.NewWindow("NYMe")
	gui.window.Resize(fyne.NewSize(600, 400))
	gui.createUI()
	gui.window.SetContent(gui.createMainUI())
	gui.window.CenterOnScreen()
	gui.window.ShowAndRun()
}

func (g *GUI) createUI() {
	g.pinEntry = widget.NewPasswordEntry()
	g.pinEntry.SetPlaceHolder("")
	g.pinEntry.Validator = func(s string) error {
		if len(s) > 8 {
			return fmt.Errorf("PIN max 8 characters")
		}
		for _, r := range s {
			if r > 127 {
				return fmt.Errorf("ASCII only")
			}
		}
		return nil
	}
	
	hashOptions := []string{"RIPEMD-256", "SHA-256", "SM3", "Streebog-256"}
	for _, opt := range hashOptions {
		currentOpt := opt
		btn := widget.NewButton(opt, func() {
			g.hashSelected = currentOpt
			for name, b := range g.hashButtons {
				if name == currentOpt {
					b.Importance = widget.HighImportance
				} else {
					b.Importance = widget.MediumImportance
				}
				b.Refresh()
			}
		})
		if opt == "RIPEMD-256" {
			btn.Importance = widget.HighImportance
		} else {
			btn.Importance = widget.MediumImportance
		}
		g.hashButtons[opt] = btn
	}
	
	g.statusLabel = widget.NewLabel("Ready")
	g.statusLabel.Wrapping = fyne.TextWrapWord
	g.filenameLabel = widget.NewLabel("No file selected")
	g.filenameLabel.TextStyle = fyne.TextStyle{Italic: true}
	g.filesizeLabel = widget.NewLabel("")
	g.filesizeLabel.TextStyle = fyne.TextStyle{Monospace: true}
	g.filesizeLabel.Hide()
	g.sigDisplay = widget.NewLabel("")
	g.sigDisplay.TextStyle = fyne.TextStyle{Italic: true}
	g.progressBar = widget.NewProgressBar()
	g.progressBar.Min = 0
	g.progressBar.Max = 1
	g.progressBar.Hide()
	g.progressLabel = widget.NewLabel("")
	g.progressLabel.Alignment = fyne.TextAlignCenter
	g.progressLabel.Hide()
	g.themeToggle = widget.NewButton("☀️", g.toggleTheme)
	g.infoBtn = widget.NewButtonWithIcon("", theme.InfoIcon(), g.showInfoPopup)
}

func (g *GUI) createMainUI() fyne.CanvasObject {
	signBtn := widget.NewButton("Sell", g.onSignClick)
	signBtn.Importance = widget.HighImportance
	verifyBtn := widget.NewButton("Verify", g.onVerifyClick)
	verifyBtn.Importance = widget.HighImportance
	buyBtn := widget.NewButton("Buy", g.onBuyClick)
	buyBtn.Importance = widget.HighImportance
	proofBtn := widget.NewButton("Proof", g.onProofClick)
	proofBtn.Importance = widget.HighImportance
	buttonContainer := container.NewCenter(container.NewHBox(signBtn, verifyBtn, buyBtn, proofBtn))
	
	topBar := container.NewHBox(g.infoBtn, layout.NewSpacer(), g.themeToggle)
	
	hashContainer := container.NewCenter(container.NewHBox(
		g.hashButtons["RIPEMD-256"],
		g.hashButtons["SHA-256"],
		g.hashButtons["SM3"],
		g.hashButtons["Streebog-256"],
	))
	
	fileInfoContainer := container.NewVBox(g.filenameLabel, g.filesizeLabel)
	clearBtn := widget.NewButton("Clear", g.onClear)
	clearBtn.Importance = widget.HighImportance
	pinClearContainer := container.NewHBox(layout.NewSpacer(), widget.NewLabel("PIN:"), g.pinEntry, clearBtn, layout.NewSpacer())
	progressContainer := container.NewVBox(g.progressLabel, g.progressBar)
	
	topContainer := container.NewVBox(
		topBar,
		widget.NewSeparator(),
		buttonContainer,
		widget.NewSeparator(),
		hashContainer,
		widget.NewSeparator(),
		fileInfoContainer,
		widget.NewSeparator(),
		g.sigDisplay,
		progressContainer,
	)
	bottomContainer := container.NewVBox(widget.NewSeparator(), pinClearContainer, g.statusLabel)
	return container.NewBorder(topContainer, bottomContainer, nil, nil)
}

func (g *GUI) toggleTheme() {
	if g.currentTheme == "dark" {
		g.app.Settings().SetTheme(&greenThemeWrapper{
			base: theme.LightTheme(),
		})
		g.currentTheme = "light"
		g.themeToggle.SetText("🌙")
	} else {
		g.app.Settings().SetTheme(&greenThemeWrapper{
			base: theme.DarkTheme(),
		})
		g.currentTheme = "dark"
		g.themeToggle.SetText("☀️")
	}
}

func (g *GUI) showInfoPopup() {
	projURL, _ := url.Parse("https://github.com/Ch1ffr3punk/NYMe")
	projectLink := widget.NewHyperlink("An Open Source project", projURL)
	okButton := widget.NewButton("OK", func() {
		overlays := g.window.Canvas().Overlays()
		if overlays.Top() != nil {
			overlays.Remove(overlays.Top())
		}
	})
	okButton.Importance = widget.HighImportance
	
	content := container.NewVBox(
		widget.NewLabelWithStyle("NYMe v0.1.0", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		container.NewHBox(layout.NewSpacer(), projectLink, layout.NewSpacer()),
		widget.NewLabelWithStyle("released under the Apache 2.0 license", fyne.TextAlignCenter, fyne.TextStyle{}),
		widget.NewLabelWithStyle("© 2026 Ch1ffr3punk", fyne.TextAlignCenter, fyne.TextStyle{}),
		container.NewHBox(layout.NewSpacer(), okButton, layout.NewSpacer()),
	)
	dialog.ShowCustomWithoutButtons("", content, g.window)
}

func (g *GUI) selectFile(callback func()) {
	previousFile := g.currentFile
	fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			g.statusLabel.SetText("Error: " + err.Error())
			return
		}
		if reader == nil {
			g.currentFile = previousFile
			return
		}
		defer reader.Close()
		g.currentFile = reader.URI().Path()
		g.filenameLabel.SetText(filepath.Base(g.currentFile))
		g.signaturePath = g.currentFile + ".sig"
		g.fileSelected = true
		fileInfo, err := os.Stat(g.currentFile)
		if err != nil {
			g.statusLabel.SetText("File info error: " + err.Error())
			return
		}
		g.filesizeLabel.SetText(fmt.Sprintf("Size: %d bytes (%s)", fileInfo.Size(), formatByteSize(int(fileInfo.Size()))))
		g.filesizeLabel.Show()
		g.statusLabel.SetText(fmt.Sprintf("Selected: %s", filepath.Base(g.currentFile)))
		if _, err := os.Stat(g.signaturePath); err == nil {
			g.sigDisplay.SetText("✓ " + filepath.Base(g.signaturePath))
		} else {
			g.sigDisplay.SetText("")
		}
		if callback != nil {
			callback()
		}
	}, g.window)
	fyne.Do(func() { fileDialog.Show() })
}

func (g *GUI) onSignClick() {
	g.selectFile(func() { g.continueSign() })
}

func (g *GUI) continueSign() {
	if g.currentFile == "" {
		dialog.ShowError(fmt.Errorf("No file selected"), g.window)
		return
	}
	if g.pinEntry.Text == "" {
		dialog.ShowError(fmt.Errorf("PIN required"), g.window)
		return
	}
	fileInfo, _ := os.Stat(g.currentFile)
	if fileInfo.Size() == 0 {
		g.statusLabel.SetText("Empty file not signable")
		return
	}
	g.statusLabel.SetText("Preparing signature...")
	g.progressBar.SetValue(0)
	g.progressBar.Show()
	g.progressLabel.Show()
	go func() {
		startTime := time.Now()
		hashAlgo := g.hashSelected
		fileHash, err := computeFileHash(g.currentFile, hashAlgo)
		if err != nil {
			g.showErrAsync("Hash error: " + err.Error())
			return
		}
		fyne.Do(func() { g.progressLabel.SetText("Signing with YubiKey...") })
		sigData, pubKey, err := signWithYubiKey([]byte(g.pinEntry.Text), fileHash)
		if err != nil {
			g.showErrAsync("Signing error: " + err.Error())
			return
		}
		hashHex := hex.EncodeToString(fileHash)
		pubKeyHex := hex.EncodeToString(pubKey)
		sigHex := hex.EncodeToString(sigData)
		
		var sigLine1, sigLine2 string
		if len(sigHex) > 64 {
			sigLine1 = sigHex[:64]
			sigLine2 = sigHex[64:]
		} else {
			sigLine1 = sigHex
			sigLine2 = ""
		}
		
		sigContent := fmt.Sprintf("%s\n%s\n%s\n%s", hashHex, pubKeyHex, sigLine1, sigLine2)
		sigFile := g.currentFile + ".sig"
		if err := os.WriteFile(sigFile, []byte(sigContent), 0644); err != nil {
			g.showErrAsync("Write error: " + err.Error())
			return
		}
		
		NYMeCode := generateNYMeCode()
		NYMeToken := formatNYMeToken(sigContent, NYMeCode)
		NYMePath := filepath.Join(filepath.Dir(g.currentFile), "NYMe.txt")
		if err := os.WriteFile(NYMePath, []byte(NYMeToken), 0644); err != nil {
			g.showErrAsync("NYMe.txt write error: " + err.Error())
			return
		}
		
		fyne.Do(func() {
			g.signaturePath = sigFile
			g.sigDisplay.SetText("✓ " + filepath.Base(sigFile))
			g.progressBar.Hide()
			g.progressLabel.Hide()
			g.statusLabel.SetText(fmt.Sprintf("✓ Signed in %.1fs: %s, NYMe.txt created", time.Since(startTime).Seconds(), filepath.Base(sigFile)))
		})
	}()
}

func computeFileHash(path, algo string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := getHasher(algo)
	buf := make([]byte, 32*1024*1024)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return h.Sum(nil), nil
}

func getHasher(algo string) hash.Hash {
	switch algo {
	case "SHA-256":
		return sha256.New()
	case "SM3":
		return sm3.New()
	case "Streebog-256":
		return gost34112012256.New()
	default:
		return ripemd.New256()
	}
}

func signWithYubiKey(pin, data []byte) ([]byte, []byte, error) {
	yk, err := openYubiKey(0)
	if err != nil {
		return nil, nil, err
	}
	defer yk.Close()
	cert, err := yk.Certificate(piv.SlotSignature)
	if err != nil {
		return nil, nil, fmt.Errorf("certificate error: %v", err)
	}
	pubKey, ok := cert.PublicKey.(ed25519.PublicKey)
	if !ok {
		return nil, nil, fmt.Errorf("no Ed25519 key in slot")
	}
	priv, err := yk.PrivateKey(piv.SlotSignature, pubKey, piv.KeyAuth{PIN: string(pin)})
	if err != nil {
		return nil, nil, fmt.Errorf("private key access: %v", err)
	}
	signer, ok := priv.(crypto.Signer)
	if !ok {
		return nil, nil, fmt.Errorf("crypto.Signer not implemented")
	}
	sig, err := signer.Sign(rand.Reader, data, crypto.Hash(0))
	if err != nil {
		return nil, nil, fmt.Errorf("signing error: %v", err)
	}
	return sig, pubKey, nil
}

func generateNYMeCode() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(b)
}

func formatNYMeToken(sigBlock, NYMeCode string) string {
	rawData := []byte(strings.TrimSpace(sigBlock) + NYMeCode)
	h := ripemd.New160()
	h.Write(rawData)
	hashStr := hex.EncodeToString(h.Sum(nil))
	
	result := "NYMe" + hashStr + NYMeCode
	
	if len(result) > 64 {
		result = result[:64]
	}
	
	return result
}

func (g *GUI) onVerifyClick() {
	g.selectFile(func() { g.continueVerify() })
}

func (g *GUI) continueVerify() {
	if !g.fileSelected {
		dialog.ShowError(fmt.Errorf("No file selected"), g.window)
		return
	}
	sigFile := g.currentFile + ".sig"
	sigData, err := os.ReadFile(sigFile)
	if err != nil {
		g.showErrAsync("Signature file not found")
		return
	}
	lines := strings.Split(strings.TrimSpace(string(sigData)), "\n")
	if len(lines) != 4 {
		g.showErrAsync("Invalid signature format (must have 4 lines)")
		return
	}
	hashHex := strings.TrimSpace(lines[0])
	pubKeyHex := strings.TrimSpace(lines[1])
	sigHex := strings.TrimSpace(lines[2]) + strings.TrimSpace(lines[3])
	
	g.statusLabel.SetText("Verifying...")
	g.progressBar.SetValue(0)
	g.progressBar.Show()
	g.progressLabel.Show()
	
	go func() {
		hashBytes, err := hex.DecodeString(hashHex)
		if err != nil {
			g.showErrAsync("Invalid hash format in signature")
			return
		}
		
		var verified bool
		var detectedAlgo string
		
		for _, algo := range []string{"RIPEMD-256", "SHA-256", "SM3", "Streebog-256"} {
			fileHash, err := computeFileHash(g.currentFile, algo)
			if err != nil {
				continue
			}
			if hex.EncodeToString(fileHash) == hashHex {
				verified = true
				detectedAlgo = algo
				break
			}
		}
		
		if !verified {
			g.showErrAsync("Hash mismatch - none of the supported hash algorithms match")
			return
		}
		
		pubKeyBytes, err := hex.DecodeString(pubKeyHex)
		if err != nil {
			g.showErrAsync("Invalid public key format")
			return
		}
		
		sigBytes, err := hex.DecodeString(sigHex)
		if err != nil {
			g.showErrAsync("Invalid signature format")
			return
		}
		
		if !ed25519.Verify(ed25519.PublicKey(pubKeyBytes), hashBytes, sigBytes) {
			g.showErrAsync("Ed25519 signature invalid")
			return
		}
		
		fyne.Do(func() {
			g.progressBar.Hide()
			g.progressLabel.Hide()
			g.statusLabel.SetText(fmt.Sprintf("Verification successful! (Hash: %s)", detectedAlgo))
			g.showIdenticonFromPubKey(pubKeyHex)
		})
	}()
}

func (g *GUI) onBuyClick() {
	sigEntry := widget.NewMultiLineEntry()
	sigEntry.SetPlaceHolder("Paste file.sig content here")
	sigEntry.SetMinRowsVisible(4)
	
	NYMeEntry := widget.NewMultiLineEntry()
	NYMeEntry.SetPlaceHolder("Paste NYMe.txt content here")
	NYMeEntry.SetMinRowsVisible(2)
	
	inputs := container.New(layout.NewFormLayout(),
		widget.NewLabel("Signature:"), sigEntry,
		widget.NewLabel("NYMe.txt:"), NYMeEntry,
	)
	
	cancelBtn := widget.NewButton("Cancel", func() {
		overlays := g.window.Canvas().Overlays()
		if overlays.Top() != nil {
			overlays.Remove(overlays.Top())
		}
	})
	
	submitBtn := widget.NewButton("Generate & Save", func() {
		sigBlock := strings.TrimSpace(sigEntry.Text)
		NYMeContent := strings.TrimSpace(NYMeEntry.Text)
		
		if sigBlock == "" {
			dialog.ShowError(fmt.Errorf("Signature content is required"), g.window)
			return
		}
		if NYMeContent == "" {
			dialog.ShowError(fmt.Errorf("NYMe.txt content is required"), g.window)
			return
		}
		
		h := ripemd.New256()
		h.Write([]byte(sigBlock + NYMeContent))
		result := hex.EncodeToString(h.Sum(nil))
		
		buyPath := "buy.txt"
		if err := os.WriteFile(buyPath, []byte(result), 0644); err != nil {
			dialog.ShowError(fmt.Errorf("Failed to save buy.txt: "+err.Error()), g.window)
			return
		}
		
		dialog.ShowInformation("Success", fmt.Sprintf("buy.txt saved to:\n%s", buyPath), g.window)
		
		overlays := g.window.Canvas().Overlays()
		if overlays.Top() != nil {
			overlays.Remove(overlays.Top())
		}
	})
	
	submitBtn.Importance = widget.HighImportance
	buttons := container.NewCenter(container.NewHBox(submitBtn, cancelBtn))
	content := container.NewVBox(inputs, widget.NewSeparator(), buttons)
	
	d := dialog.NewCustomWithoutButtons("Buy Token", content, g.window)
	d.Resize(fyne.NewSize(500, 400))
	fyne.Do(func() { d.Show() })
}

func (g *GUI) onProofClick() {
	g.selectFile(func() {
		if !g.fileSelected {
			return
		}
		g.continueProof()
	})
}

func (g *GUI) continueProof() {
	sigFile := g.currentFile + ".sig"
	buyFile := filepath.Join(filepath.Dir(g.currentFile), "buy.txt")
	NYMeFile := filepath.Join(filepath.Dir(g.currentFile), "NYMe.txt")
	if _, err := os.Stat(sigFile); err != nil {
		g.showErrAsync(".sig missing")
		return
	}
	if _, err := os.Stat(NYMeFile); err != nil {
		g.showErrAsync("NYMe.txt missing")
		return
	}
	if _, err := os.Stat(buyFile); err != nil {
		g.showErrAsync("buy.txt missing")
		return
	}
	g.statusLabel.SetText("Verifying proof...")
	go func() {
		sigData, _ := os.ReadFile(sigFile)
		lines := strings.Split(strings.TrimSpace(string(sigData)), "\n")
		if len(lines) != 4 {
			g.showErrAsync("Invalid signature format")
			return
		}
		hashHex := strings.TrimSpace(lines[0])
		pubKeyHex := strings.TrimSpace(lines[1])
		sigHex := strings.TrimSpace(lines[2]) + strings.TrimSpace(lines[3])
		
		hashBytes, err := hex.DecodeString(hashHex)
		if err != nil {
			g.showErrAsync("Invalid hash format")
			return
		}
		
		var verified bool
		var detectedAlgo string
		
		for _, algo := range []string{"RIPEMD-256", "SHA-256", "SM3", "Streebog-256"} {
			fileHash, err := computeFileHash(g.currentFile, algo)
			if err != nil {
				continue
			}
			if hex.EncodeToString(fileHash) == hashHex {
				verified = true
				detectedAlgo = algo
				break
			}
		}
		
		if !verified {
			g.showErrAsync("Hash mismatch")
			return
		}
		
		pubKeyBytes, _ := hex.DecodeString(pubKeyHex)
		sigBytes, _ := hex.DecodeString(sigHex)
		if !ed25519.Verify(ed25519.PublicKey(pubKeyBytes), hashBytes, sigBytes) {
			g.showErrAsync("Signature invalid")
			return
		}
		
		NYMeData, _ := os.ReadFile(NYMeFile)
		NYMeContent := strings.TrimSpace(string(NYMeData))
		if len(NYMeContent) != 64 || !strings.HasPrefix(NYMeContent, "NYMe") {
			g.showErrAsync("NYMe.txt format invalid")
			return
		}
		
		h := ripemd.New256()
		h.Write(append([]byte(strings.TrimSpace(string(sigData))), []byte(NYMeContent)...))
		expectedBuy := hex.EncodeToString(h.Sum(nil))
		buyData, err := os.ReadFile(buyFile)
		if err != nil {
			g.showErrAsync("buy.txt missing")
			return
		}
		if strings.TrimSpace(string(buyData)) != expectedBuy {
			g.showErrAsync("buy.txt mismatch")
			return
		}
		fyne.Do(func() {
			g.statusLabel.SetText(fmt.Sprintf("✓ Proof verified successfully (Hash: %s)", detectedAlgo))
			g.showIdenticonFromPubKey(pubKeyHex)
		})
	}()
}

func (g *GUI) showIdenticonFromPubKey(pubKeyHex string) {
	hash := sha256.Sum256([]byte(pubKeyHex))
	identicon := NewClassicIdenticon(hash[:])
	img := identicon.Generate()
	fyneImg := canvas.NewImageFromImage(img)
	fyneImg.FillMode = canvas.ImageFillContain
	fyneImg.SetMinSize(fyne.NewSize(128, 128))
	
	okBtn := widget.NewButton("OK", func() {
		if overlays := g.window.Canvas().Overlays(); overlays.Top() != nil {
			overlays.Remove(overlays.Top())
		}
	})
	okBtn.Importance = widget.HighImportance
	
	content := container.NewVBox(
		container.NewCenter(fyneImg),
		container.NewCenter(widget.NewLabel("Verification successful!")),
		container.NewCenter(okBtn),
	)
	
	d := dialog.NewCustomWithoutButtons("", content, g.window)
	fyne.Do(func() { d.Show() })
}

func (g *GUI) showErrAsync(msg string) {
	fyne.Do(func() {
		g.progressBar.Hide()
		g.progressLabel.Hide()
		dialog.ShowError(fmt.Errorf(msg), g.window)
		g.statusLabel.SetText("Ready")
	})
}

func (g *GUI) onClear() {
	g.pinEntry.SetText("")
	g.currentFile = ""
	g.signaturePath = ""
	g.fileSelected = false
	g.filenameLabel.SetText("No file selected")
	g.filesizeLabel.SetText("")
	g.filesizeLabel.Hide()
	g.sigDisplay.SetText("")
	g.progressBar.Hide()
	g.progressLabel.Hide()
	g.statusLabel.SetText("Ready")
	g.hashSelected = "RIPEMD-256"
	for name, btn := range g.hashButtons {
		if name == "RIPEMD-256" {
			btn.Importance = widget.HighImportance
		} else {
			btn.Importance = widget.MediumImportance
		}
		btn.Refresh()
	}
}

func openYubiKey(index int) (*piv.YubiKey, error) {
	cards, err := piv.Cards()
	if err != nil {
		return nil, err
	}
	count := 0
	for _, card := range cards {
		if strings.Contains(strings.ToLower(card), "yubikey") {
			if count == index {
				return piv.Open(card)
			}
			count++
		}
	}
	return nil, fmt.Errorf("YubiKey not found")
}

func formatByteSize(bytes int) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

type ClassicIdenticon struct {
	source []byte
	size   int
}

func NewClassicIdenticon(source []byte) *ClassicIdenticon {
	return &ClassicIdenticon{source: source, size: 256}
}

func (i *ClassicIdenticon) getBit(n int) bool {
	if len(i.source) == 0 || n < 0 {
		return false
	}
	b, bit := n/8, n%8
	if b >= len(i.source) {
		return false
	}
	return (i.source[b]>>bit)&1 == 1
}

func (i *ClassicIdenticon) foreground() color.Color {
	if len(i.source) < 32 {
		return color.RGBA{0, 0, 0, 255}
	}
	idx := 0
	for j := 0; j < 4; j++ {
		if i.getBit(248 + j) {
			idx |= 1 << j
		}
	}
	idx %= 16
	palette := []color.RGBA{
		{0x00, 0xbf, 0x93, 0xff}, {0x2d, 0xcc, 0x70, 0xff}, {0x42, 0xe4, 0x53, 0xff}, {0xf1, 0xc4, 0x0f, 0xff},
		{0xe6, 0x7f, 0x22, 0xff}, {0xff, 0x94, 0x4e, 0xff}, {0xe8, 0x4c, 0x3d, 0xff}, {0x35, 0x98, 0xdb, 0xff},
		{0x9a, 0x59, 0xb5, 0xff}, {0xef, 0x3e, 0x96, 0xff}, {0xdf, 0x21, 0xb9, 0xff}, {0x7d, 0xc2, 0xd2, 0xff},
		{0x16, 0xa0, 0x86, 0xff}, {0x27, 0xae, 0x61, 0xff}, {0x24, 0xc3, 0x33, 0xff}, {0x1c, 0xab, 0xbb, 0xff},
	}
	return palette[idx]
}

func (i *ClassicIdenticon) secondaryColor() color.Color {
	if len(i.source) < 32 {
		return color.RGBA{100, 100, 100, 255}
	}
	idx := 0
	for j := 0; j < 4; j++ {
		if i.getBit(244 + j) {
			idx |= 1 << j
		}
	}
	idx %= 16
	palette := []color.RGBA{
		{0x34, 0x49, 0x5e, 0xff}, {0x95, 0xa5, 0xa5, 0xff}, {0xd2, 0x54, 0x00, 0xff}, {0xc1, 0x39, 0x2b, 0xff},
		{0x29, 0x7f, 0xb8, 0xff}, {0x8d, 0x44, 0xad, 0xff}, {0xbe, 0x12, 0x7e, 0xff}, {0xe5, 0x23, 0x83, 0xff},
		{0x27, 0xae, 0x61, 0xff}, {0x24, 0xc3, 0x33, 0xff}, {0xd9, 0xd9, 0x21, 0xff}, {0xf3, 0x9c, 0x11, 0xff},
		{0xff, 0x55, 0x00, 0xff}, {0x1c, 0xab, 0xbb, 0xff}, {0x23, 0x23, 0x23, 0xff}, {0x7e, 0x8c, 0x8d, 0xff},
	}
	return palette[idx]
}

func (i *ClassicIdenticon) generatePixelPattern() ([]bool, []bool) {
	primary, secondary := make([]bool, 25), make([]bool, 25)
	b := 0
	for r := 0; r < 5; r++ {
		for c := 0; c < 3; c++ {
			p := i.getBit(b)
			b++
			primary[r*5+c] = p
			primary[r*5+(4-c)] = p
		}
	}
	for r := 0; r < 5; r++ {
		for c := 0; c < 3; c++ {
			p := i.getBit(b)
			b++
			secondary[r*5+c] = p
			secondary[r*5+(4-c)] = p
		}
	}
	return primary, secondary
}

func (i *ClassicIdenticon) drawRect(img *image.RGBA, x0, y0, x1, y1 int, c color.Color) {
	r, g, b, a := c.RGBA()
	rgba := color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			img.SetRGBA(x, y, rgba)
		}
	}
}

func (i *ClassicIdenticon) Generate() image.Image {
	const pixelSize = 36
	const spriteSize = 5
	margin := (256 - pixelSize*spriteSize) / 2
	bgChoice := 0
	for j := 0; j < 2; j++ {
		if i.getBit(252 + j) {
			bgChoice |= 1 << j
		}
	}
	bgChoice %= 3
	var bg color.RGBA
	if fyne.CurrentApp().Settings().ThemeVariant() == theme.VariantDark {
		bg = []color.RGBA{{30, 30, 30, 255}, {45, 62, 80, 255}, {57, 57, 57, 255}}[bgChoice]
	} else {
		bg = []color.RGBA{{255, 255, 255, 255}, {243, 245, 247, 255}, {236, 240, 241, 255}}[bgChoice]
	}
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			img.SetRGBA(x, y, bg)
			img.SetRGBA(x, y, bg)
		}
	}
	p, s := i.generatePixelPattern()
	for r := 0; r < spriteSize; r++ {
		for c := 0; c < spriteSize; c++ {
			if s[r*spriteSize+c] {
				i.drawRect(img, c*pixelSize+margin, r*pixelSize+margin, (c+1)*pixelSize+margin, (r+1)*pixelSize+margin, i.secondaryColor())
			}
			if p[r*spriteSize+c] {
				i.drawRect(img, c*pixelSize+margin, r*pixelSize+margin, (c+1)*pixelSize+margin, (r+1)*pixelSize+margin, i.foreground())
			}
		}
	}
	return img
}

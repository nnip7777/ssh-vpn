package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var (
	neonCyan    = color.NRGBA{R: 0, G: 212, B: 255, A: 255}
	neonMagenta = color.NRGBA{R: 224, G: 64, B: 251, A: 255}
	neonGreen   = color.NRGBA{R: 0, G: 255, B: 136, A: 255}
	darkBg      = color.NRGBA{R: 10, G: 14, B: 26, A: 255}
	darkPanel   = color.NRGBA{R: 17, G: 24, B: 39, A: 255}
	darkBorder  = color.NRGBA{R: 45, G: 55, B: 72, A: 255}
	darkInput   = color.NRGBA{R: 15, G: 20, B: 35, A: 255}
	textWhite   = color.NRGBA{R: 230, G: 237, B: 243, A: 255}
	textGrey    = color.NRGBA{R: 156, G: 163, B: 175, A: 255}
	dangerRed   = color.NRGBA{R: 239, G: 68, B: 68, A: 255}
	cyanDim     = color.NRGBA{R: 0, G: 140, B: 180, A: 255}
	cyanGlow    = color.NRGBA{R: 0, G: 212, B: 255, A: 30}
	neonBorder  = color.NRGBA{R: 0, G: 180, B: 220, A: 180}
)

type NeonTheme struct{}

func (n *NeonTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return darkBg
	case theme.ColorNameButton:
		return darkPanel
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 20, G: 28, B: 45, A: 255}
	case theme.ColorNamePrimary:
		return neonCyan
	case theme.ColorNameForeground:
		return textWhite
	case theme.ColorNamePlaceHolder:
		return textGrey
	case theme.ColorNameHover:
		return color.NRGBA{R: 0, G: 212, B: 255, A: 40}
	case theme.ColorNameFocus:
		return neonCyan
	case theme.ColorNamePressed:
		return color.NRGBA{R: 0, G: 180, B: 220, A: 255}
	case theme.ColorNameDisabled:
		return textGrey
	case theme.ColorNameError:
		return dangerRed
	case theme.ColorNameSuccess:
		return neonGreen
	case theme.ColorNameWarning:
		return neonMagenta
	case theme.ColorNameSeparator:
		return darkBorder
	case theme.ColorNameOverlayBackground:
		return darkPanel
	case theme.ColorNameHeaderBackground:
		return color.NRGBA{R: 13, G: 17, B: 30, A: 255}
	case theme.ColorNameMenuBackground:
		return darkPanel
	case theme.ColorNameInputBackground:
		return darkInput
	case theme.ColorNameScrollBar:
		return darkBorder
	case theme.ColorNameSelection:
		return color.NRGBA{R: 0, G: 212, B: 255, A: 60}
	default:
		return theme.DefaultTheme().Color(name, variant)
	}
}

func (n *NeonTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (n *NeonTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (n *NeonTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameText:
		return 14
	case theme.SizeNameSubHeadingText:
		return 18
	case theme.SizeNameHeadingText:
		return 24
	case theme.SizeNamePadding:
		return 10
	case theme.SizeNameInnerPadding:
		return 8
	case theme.SizeNameSeparatorThickness:
		return 1
	default:
		return theme.DefaultTheme().Size(name)
	}
}

type NeonPanel struct {
	widget.BaseWidget
	title   string
	content fyne.CanvasObject
	border  color.Color
}

func NewNeonPanel(title string, content fyne.CanvasObject) *NeonPanel {
	p := &NeonPanel{title: title, content: content, border: neonBorder}
	p.ExtendBaseWidget(p)
	return p
}

func (p *NeonPanel) CreateRenderer() fyne.WidgetRenderer {
	titleLabel := widget.NewLabel(p.title)
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}
	titleLabel.Importance = widget.HighImportance

	bg := canvas.NewRectangle(darkPanel)
	borderRect := canvas.NewRectangle(p.border)

	innerContent := container.NewBorder(
		container.NewVBox(titleLabel, widget.NewSeparator()),
		nil, nil, nil,
		p.content,
	)

	innerBg := canvas.NewRectangle(darkPanel)

	return &neonPanelRenderer{
		panel:      p,
		bg:         bg,
		border:     borderRect,
		innerBg:    innerBg,
		content:    innerContent,
		titleLabel: titleLabel,
	}
}

type neonPanelRenderer struct {
	panel      *NeonPanel
	bg         *canvas.Rectangle
	border     *canvas.Rectangle
	innerBg    *canvas.Rectangle
	content    *fyne.Container
	titleLabel *widget.Label
}

func (r *neonPanelRenderer) Destroy() {}
func (r *neonPanelRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	r.border.Resize(size)
	r.border.Move(fyne.NewPos(1, 1))
	r.border.Resize(fyne.NewSize(size.Width-2, size.Height-2))
	r.innerBg.Resize(fyne.NewSize(size.Width-4, size.Height-4))
	r.innerBg.Move(fyne.NewPos(2, 2))
	r.content.Resize(fyne.NewSize(size.Width-4, size.Height-4))
	r.content.Move(fyne.NewPos(2, 2))
}
func (r *neonPanelRenderer) MinSize() fyne.Size { return r.content.MinSize() }
func (r *neonPanelRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.border, r.innerBg, r.content}
}
func (r *neonPanelRenderer) Refresh() { r.content.Refresh() }

func CreateHeaderBar(title string) fyne.CanvasObject {
	titleLabel := widget.NewLabel(title)
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}

	logo := canvas.NewText("SSH VPN", neonCyan)
	logo.TextSize = 20
	logo.TextStyle = fyne.TextStyle{Bold: true}

	versionText := canvas.NewText("v0.4.0", textGrey)
	versionText.TextSize = 12

	left := container.NewHBox(logo, versionText)
	right := container.NewHBox(titleLabel)

	return container.NewBorder(nil, nil, left, right,
		canvas.NewRectangle(color.NRGBA{R: 13, G: 17, B: 30, A: 255}))
}

func CreateStatusBar(statusItems ...string) fyne.CanvasObject {
	objects := []fyne.CanvasObject{}
	for i, item := range statusItems {
		t := canvas.NewText(item, textGrey)
		t.TextSize = 11
		objects = append(objects, t)
		if i < len(statusItems)-1 {
			objects = append(objects, layout.NewSpacer())
		}
	}
	bar := container.NewHBox(objects...)
	return container.NewBorder(
		canvas.NewRectangle(color.NRGBA{R: 13, G: 17, B: 30, A: 255}),
		nil, nil, nil, bar)
}

func CreateStatCard(label, value string, accent color.Color) fyne.CanvasObject {
	titleText := canvas.NewText(label, textGrey)
	titleText.TextSize = 11

	valText := canvas.NewText(value, accent)
	valText.TextSize = 22
	valText.TextStyle = fyne.TextStyle{Bold: true}

	card := container.NewVBox(titleText, valText)
	bg := canvas.NewRectangle(darkPanel)

	return container.NewStack(bg, container.NewPadded(card))
}

func CreateNeonButton(text string, accent color.Color, fn func()) *widget.Button {
	btn := widget.NewButton(text, fn)
	btn.Importance = widget.HighImportance
	return btn
}

func CreateTitledSeparator(title string) fyne.CanvasObject {
	label := canvas.NewText(title, cyanDim)
	label.TextSize = 11
	label.TextStyle = fyne.TextStyle{Bold: true}
	return container.NewHBox(label, widget.NewSeparator())
}

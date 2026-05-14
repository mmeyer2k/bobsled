// internal/tui/forms.go
package tui

import "github.com/charmbracelet/huh"

// FormWithResult pairs a huh form with a closure that reads the bound result
// value after the form completes. The closure captures a pointer to the
// stack-allocated value bound via .Value(&v), so it is reliable regardless of
// which getter methods huh exposes in the installed version.
type FormWithResult struct {
	Form   *huh.Form
	Result func() interface{}
}

// NewConfirmForm builds a Yes/No confirmation huh form. After the form
// completes, call Result() and type-assert to bool; true means the user
// chose Yes.
func NewConfirmForm(title, body string) FormWithResult {
	v := false
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(title).
			Description(body).
			Affirmative("Yes").
			Negative("Cancel").
			Value(&v),
	))
	return FormWithResult{Form: form, Result: func() interface{} { return v }}
}

// NewInputForm builds a single-line text-input huh form. After the form
// completes, call Result() and type-assert to string.
func NewInputForm(title, body, placeholder string) FormWithResult {
	v := ""
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title(title).
			Description(body).
			Placeholder(placeholder).
			Value(&v),
	))
	return FormWithResult{Form: form, Result: func() interface{} { return v }}
}

// NewMultiSelectForm builds a filterable multi-select huh form. After the form
// completes, call Result() and type-assert to []string.
func NewMultiSelectForm(title, body string, items []string) FormWithResult {
	opts := make([]huh.Option[string], 0, len(items))
	for _, it := range items {
		opts = append(opts, huh.NewOption(it, it))
	}
	v := []string{}
	form := huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title(title).
			Description(body).
			Options(opts...).
			Filterable(true).
			Value(&v),
	))
	return FormWithResult{Form: form, Result: func() interface{} { return v }}
}

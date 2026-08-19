package flux

import (
	"fmt"
	"sort"
	"strings"
)

// Locale identifies an application locale. Values are opaque and case-sensitive;
// applications commonly use BCP 47 tags such as "en" and "zh-CN".
type Locale string

// MessageID is a stable, language-independent message key.
type MessageID string

// Messages maps message IDs to printf-style message templates for one locale.
type Messages map[MessageID]string

// Resources maps locales to their messages.
type Resources map[Locale]Messages

// ErrInvalidCatalog is returned when NewCatalog receives invalid resources.
var ErrInvalidCatalog = newDiagnosticError(DiagnosticErrInvalidCatalog)

// Catalog is an immutable set of localized resources with one fallback locale.
// Lookups first use the requested locale, then the fallback locale.
//
// Construct a Catalog with NewCatalog. Its resource maps are never exposed
// directly, so a Catalog is safe for concurrent lookups.
type Catalog struct {
	fallback  Locale
	resources Resources
}

// NewCatalog validates and defensively copies resources. fallback must be
// non-empty and present in resources; locale and message ID keys must also be
// non-empty. Validation errors have deterministic ordering.
//
// The caller retains ownership of resources and may mutate or replace any of
// its maps after this function returns.
func NewCatalog(fallback Locale, resources Resources) (*Catalog, error) {
	issues := validateResources(fallback, resources)
	if len(issues) != 0 {
		return nil, fmt.Errorf("%w: %s", ErrInvalidCatalog, strings.Join(issues, "; "))
	}
	return &Catalog{fallback: fallback, resources: cloneResources(resources)}, nil
}

// MustCatalog is NewCatalog for package-level or embedded resource setup. It
// panics with the validation error when resources are invalid.
func MustCatalog(fallback Locale, resources Resources) *Catalog {
	catalog, err := NewCatalog(fallback, resources)
	if err != nil {
		panic(err)
	}
	return catalog
}

// Fallback returns the catalog's fallback locale.
func (c *Catalog) Fallback() Locale { return c.fallback }

// Resources returns a defensive copy of every locale and message map. The
// returned maps may be freely changed or replaced by the caller.
func (c *Catalog) Resources() Resources { return cloneResources(c.resources) }

// Lookup returns the message template for id. It checks locale first and the
// catalog fallback second. An empty translation is a present translation.
func (c *Catalog) Lookup(locale Locale, id MessageID) (string, bool) {
	if messages, ok := c.resources[locale]; ok {
		if message, ok := messages[id]; ok {
			return message, true
		}
	}
	if locale != c.fallback {
		if message, ok := c.resources[c.fallback][id]; ok {
			return message, true
		}
	}
	return "", false
}

// Format looks up id and applies fmt.Sprintf when args are supplied. If id is
// missing from both locales, Format returns the message ID so missing UI text
// remains visible and deterministic.
func (c *Catalog) Format(locale Locale, id MessageID, args ...any) string {
	message, ok := c.Lookup(locale, id)
	if !ok {
		return string(id)
	}
	if len(args) == 0 {
		return message
	}
	return fmt.Sprintf(message, args...)
}

// MessageBinding is a read-only localized text binding. Pass it to Text,
// Button, CheckBox, RadioButton, or Memo. Changing its locale State causes the
// owning App to render and patch the existing control.
type MessageBinding struct {
	catalog *Catalog
	locale  *State[Locale]
	id      MessageID
	args    []any
}

// Bind creates a read-only binding for a localized message. It panics if
// locale is nil. Formatting arguments are copied when the binding is created.
func (c *Catalog) Bind(locale *State[Locale], id MessageID, args ...any) *MessageBinding {
	if locale == nil {
		panic(DiagnosticText(DiagnosticCatalogNilLocale))
	}
	return &MessageBinding{
		catalog: c,
		locale:  locale,
		id:      id,
		args:    append([]any(nil), args...),
	}
}

func (b *MessageBinding) renderText() string {
	return b.catalog.Format(b.locale.Get(), b.id, b.args...)
}

func (b *MessageBinding) onChange() func(string) { return nil }

func (b *MessageBinding) subscription() stateSubscription { return b.locale }

func validateResources(fallback Locale, resources Resources) []string {
	var issues []string
	if fallback == "" {
		issues = append(issues, DiagnosticText(DiagnosticCatalogFallbackEmpty))
	}
	if len(resources) == 0 {
		issues = append(issues, DiagnosticText(DiagnosticCatalogResourcesEmpty))
	}
	if fallback != "" {
		if _, ok := resources[fallback]; !ok {
			issues = append(issues, DiagnosticText(DiagnosticCatalogFallbackMissing, fallback))
		}
	}

	locales := make([]Locale, 0, len(resources))
	for locale := range resources {
		locales = append(locales, locale)
	}
	sort.Slice(locales, func(i, j int) bool { return locales[i] < locales[j] })
	for _, locale := range locales {
		if locale == "" {
			issues = append(issues, DiagnosticText(DiagnosticCatalogLocaleEmpty))
		}
		ids := make([]MessageID, 0, len(resources[locale]))
		for id := range resources[locale] {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			if id == "" {
				issues = append(issues, DiagnosticText(DiagnosticCatalogMessageIDEmpty, locale))
			}
		}
	}
	return issues
}

func cloneResources(resources Resources) Resources {
	cloned := make(Resources, len(resources))
	for locale, messages := range resources {
		messageCopy := make(Messages, len(messages))
		for id, message := range messages {
			messageCopy[id] = message
		}
		cloned[locale] = messageCopy
	}
	return cloned
}

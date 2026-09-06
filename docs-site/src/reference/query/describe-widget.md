# DESCRIBE WIDGET

## Synopsis

    DESCRIBE WIDGET <keyword>

    DESCRIBE WIDGET '<widget id>'

## Description

Shows the format mxcli has discovered for a pluggable or custom widget: its
properties (key, type, caption, category, required, default, enumeration
members), the **body containers** it accepts, the editor rules that hide a
property under some configurations, and a complete MDL example.

A widget was the only MDL extension point without a `DESCRIBE`. That is why
`mxcli widget init` writes markdown documentation at all — and why the two could
drift. `DESCRIBE WIDGET` and `mxcli widget describe` are the same function, so
they cannot disagree.

Unlike the other `DESCRIBE` statements, this one **works with no project open**:
"what can I write here?" is a question asked before anything is open. With `-p`,
the properties and rules come from the widget package actually installed in the
project (`widgets/*.mpk`) — version-accurate, and the only place a Marketplace
widget appears at all. Without it, they come from mxcli's embedded template.

## Parameters

*keyword*
: The widget's MDL name, as written in a page body — `combobox`, `htmlelement`,
  `datagrid`. Case-insensitive.

*widget id*
: The full widget id as a quoted string —
  `'com.mendix.widget.web.htmlelement.HTMLElement'`. Use this form for a widget
  whose MDL name is ambiguous, or when you have the id in hand from a `.mpk`.

## Examples

```sql
DESCRIBE WIDGET htmlelement;
```

Example output, abbreviated:

```
Widget: HTML Element (htmlelement)
  ID:      com.mendix.widget.web.htmlelement.HTMLElement
  Version: 1.2.2
  Kind:    pluggable
  Source:  project .mpk

Properties (23):
  tagName                    enumeration required default=div {div|span|p|ul|…}
  tagContentMode             enumeration required default=container {container|innerHTML}
  attributes                 object (General::HTML attributes)
    attributeName              string required
    attributeValueType         enumeration required default=expression {expression|template}

Body containers (4):
  attribute                  object list  -> attributes                 authorable
                               items: attributeName, attributeValueType, …
  event                      object list  -> events                     authorable
                               items: eventName, eventAction, …
  tagcontentcontainer        child slot   -> tagContentContainer        authorable
  tagcontentrepeatcontainer  child slot   -> tagContentRepeatContainer  authorable

MDL example (parses as written):
  htmlelement widget1 (
    tagName: 'div',
    tagUseRepeat: false,
    tagContentMode: 'container'
  ) {
    attribute item1 (attributeValueType: 'expression')   -- one entry of `attributes`
    event item2 (eventName: 'onClick')   -- one entry of `events`
    tagcontentcontainer slot3 {
      -- widgets for `tagContentContainer`
    }
  }
```

By widget id:

```sql
DESCRIBE WIDGET 'com.mendix.widget.web.htmlelement.HTMLElement';
```

## Notes

**Body containers report whether MDL can express them.** `authorable` is derived
by parsing a probe against the live grammar, never read from a list — so the mark
is correct by construction rather than by maintenance. A container reported as
not authorable is one to set in Studio Pro.

**The MDL example parses and checks as written.** The whole block is parsed
before it is emitted, and its property values are real members rather than
placeholders. Bindings it cannot fill — a datasource, an attribute, an action —
are **named rather than invented**, under `-- omitted:`, because a generic
example cannot know a name from your project.

**The example is narrowed by the widget's own editor rules.** A property the
widget hides under the configuration the example picked is left out, so what you
see is what that configuration actually supports. The footer reports how many of
the widget's hide-rules were recognised; an unrecognised rule never prunes.

**`LIST WIDGETS` does not exist**, deliberately. `SHOW WIDGETS` already means
widget *instances placed on pages*, and the definitions are
`SELECT * FROM CATALOG.WIDGET_DEFINITIONS`.

## See Also

[SHOW WIDGETS](show-widgets.md), [DESCRIBE PAGE](describe-page.md),
[Pluggable Widgets Across Versions](../../guides/pluggable-widgets.md)

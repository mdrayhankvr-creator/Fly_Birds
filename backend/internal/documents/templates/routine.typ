// Exam routine documents (E8): a class's routine, a room's schedule, or the
// invigilation duties. One page group per class or room; long groups flow
// onto more pages with the table header repeated.
#import "common.typ": *
#show: base

#let L = doc.labels
#set page(
  paper: "a4",
  flipped: doc.landscape,
  margin: (x: 12mm, top: 11mm, bottom: 14mm),
  foreground: watermark,
  footer: context {
    set text(size: 6.5pt, fill: muted)
    grid(columns: (1fr, auto), doc.footer.join(" · "), [#L.page #counter(page).display() / #counter(page).final().first()])
  },
)

#let th(s) = text(size: 7.5pt, weight: "bold", fill: muted, s)
#let lines = (x, y) => (bottom: if y == 0 { 0.8pt + accent.lighten(30%) } else { 0.4pt + rule })

#let header(page) = {
  grid(columns: (17mm, 1fr, 17mm), column-gutter: 3mm, align: (left + horizon, center + horizon, right + horizon),
    crest(16mm), school-lines(), [])
  v(2mm)
  provisional-banner()
  block(width: 100%, fill: tint, stroke: (left: 3pt + accent, rest: 0.5pt + accent.lighten(55%)), inset: (x: 4mm, y: 2.2mm), radius: 2pt,
    grid(columns: (1fr, auto), align: (left + horizon, right + horizon), column-gutter: 3mm,
      { text(size: 11.5pt, weight: "bold", fill: accent.darken(25%), doc.heading); linebreak(); text(size: 8.5pt, weight: "semibold", doc.subtitle) },
      { text(size: 11pt, weight: "bold", page.heading); if page.note != "" { linebreak(); text(size: 7.5pt, fill: muted, page.note) } }))
  v(2.5mm)
}

#let cell(s, bold: false) = {
  let parts = s.split("\n")
  for (i, p) in parts.enumerate() {
    if i == 0 { text(weight: if bold { "semibold" } else { "regular" }, p) } else { linebreak(); text(size: 7.5pt, fill: muted, p) }
  }
}

#for (n, page) in doc.pages.enumerate() {
  if n > 0 { pagebreak() }
  header(page)
  if page.rows.len() == 0 {
    block(width: 100%, inset: 6mm, stroke: 0.6pt + rule, radius: 3pt, align(center, text(fill: muted, page.empty)))
  } else {
    set text(size: 8.5pt)
    table(
      columns: page.columns.map(c => c * 1fr),
      stroke: lines,
      fill: (x, y) => if y == 0 { tint } else { none },
      inset: (x: 1.8mm, y: 1.9mm),
      align: (x, y) => if page.centered.contains(x) { center + horizon } else { left + horizon },
      table.header(..page.headers.map(th)),
      ..page.rows.enumerate().map(((i, r)) => r.enumerate().map(((x, c)) => {
        let top = if page.shade.at(i) and i > 0 { (top: 0.9pt + accent.lighten(45%)) } else { (:) }
        table.cell(stroke: top, cell(c, bold: x == 0 or (doc.kind == "class" and x == 2)))
      })).flatten(),
    )
  }
  if doc.notes.len() > 0 {
    v(3mm)
    set text(size: 7.5pt, fill: muted)
    for note in doc.notes { par(note) }
  }
}

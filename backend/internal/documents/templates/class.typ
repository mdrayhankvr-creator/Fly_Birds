// Class result documents: tabulation sheet (landscape), merit list,
// failed-subjects list and result summary.
#import "common.typ": *
#show: base

#let L = doc.labels
#let landscape = doc.kind == "tabulation"
#set page(
  paper: if doc.paper == "a3" { "a3" } else { "a4" },
  flipped: landscape,
  margin: (x: 11mm, top: 10mm, bottom: 13mm),
  foreground: watermark,
  footer: context {
    set text(size: 6.5pt, fill: muted)
    grid(columns: (1fr, auto), doc.footer, [#L.page #counter(page).display() / #counter(page).final().first()])
  },
)

#let heading-block() = {
  grid(columns: (16mm, 1fr, 16mm), column-gutter: 3mm, align: (left + horizon, center + horizon, right + horizon),
    crest(15mm), school-lines(), [])
  v(2mm)
  provisional-banner()
  block(width: 100%, fill: tint, stroke: (left: 3pt + accent, rest: 0.5pt + accent.lighten(55%)), inset: (x: 4mm, y: 2.2mm), radius: 2pt,
    grid(columns: (1fr, auto), align: (left + horizon, right + horizon), column-gutter: 3mm,
      text(size: 11.5pt, weight: "bold", fill: accent.darken(25%), doc.heading),
      text(size: 8.5pt, weight: "semibold", doc.subtitle)))
  v(2.5mm)
}

#let th(s) = text(size: 7pt, weight: "bold", fill: muted, s)
#let zebra = (x, y) => if y == 0 { tint } else if calc.even(y) { rgb("#f8fafc") } else { none }
#let lines = (x, y) => (bottom: if y == 0 { 0.8pt + accent.lighten(30%) } else { 0.4pt + rule })

#heading-block()

#if doc.kind == "tabulation" {
  let T = doc.tabulation
  let n = T.subjects.len()
  set text(size: if n > 10 { 7pt } else { 7.5pt })
  table(
    columns: (9mm, 38mm, ..T.subjects.map(_ => 1fr), 14mm, 11mm, 10mm, 15mm),
    stroke: lines, fill: zebra, inset: (x: 1mm, y: 1.2mm),
    align: (x, y) => if x == 1 { left + horizon } else { center + horizon },
    table.header(th(L.roll), th(L.name), ..T.subjects.map(s => th({ s.name; if s.code != "" { linebreak(); text(fill: faint, s.code) } })),
      th(L.total), th(L.gpa), th(L.position), th(L.result)),
    ..T.rows.map(r => (
      r.roll,
      { text(weight: "semibold", r.name); if r.section != "" { linebreak(); text(size: 6pt, fill: muted, r.section) } },
      ..r.cells.map(c => if c.marks == "" { text(fill: faint, "—") } else {
        text(fill: if c.failed { fail-red } else { ink }, { c.marks; linebreak(); text(size: 6pt, weight: "bold", c.grade) })
      }),
      text(weight: "bold", r.total), text(weight: "bold", r.gpa), r.position,
      text(weight: "semibold", fill: if r.failed { fail-red } else { ink }, r.result),
    )).flatten(),
  )
} else if doc.kind == "merit_list" {
  table(
    columns: (16mm, 14mm, 1fr, 26mm, 22mm, 18mm, 18mm),
    stroke: lines, fill: zebra, inset: (x: 1.6mm, y: 1.8mm),
    align: (x, y) => if x == 2 { left + horizon } else { center + horizon },
    table.header(th(L.position), th(L.roll), th(L.name), th(L.section), th(L.total), th(L.gpa), th(L.letter)),
    ..doc.merit.map(r => (text(weight: "bold", r.position), r.roll, { text(weight: "semibold", r.name); if r.name_alt != "" { linebreak(); text(size: 6.5pt, fill: muted, r.name_alt) } },
      r.section, r.total, text(weight: "bold", r.gpa), r.letter)).flatten(),
  )
} else if doc.kind == "failed_list" {
  if doc.failed.len() == 0 {
    block(width: 100%, inset: 6mm, stroke: 0.6pt + rule, radius: 3pt, align(center, text(fill: muted, L.no_failures)))
  } else {
    table(
      columns: (14mm, 1fr, 24mm, 1.4fr),
      stroke: lines, fill: zebra, inset: (x: 1.6mm, y: 1.8mm),
      align: (x, y) => if x == 0 or x == 2 { center + horizon } else { left + horizon },
      table.header(th(L.roll), th(L.name), th(L.section), th(L.subjects_failed)),
      ..doc.failed.map(r => (r.roll, text(weight: "semibold", r.name), r.section, text(fill: fail-red, r.subjects))).flatten(),
    )
  }
} else if doc.kind == "summary" {
  let S = doc.summary
  let tile(k, v, strong: false) = block(width: 100%, inset: (x: 3mm, y: 2.4mm), radius: 2pt,
    stroke: if strong { 1pt + accent } else { 0.5pt + rule }, fill: if strong { tint } else { none },
    { text(size: 7pt, fill: muted, k); linebreak(); text(size: if strong { 15pt } else { 12pt }, weight: "bold", v) })
  grid(columns: (1fr,) * S.tiles.len(), column-gutter: 2.5mm, ..S.tiles.map(t => tile(t.label, t.value, strong: t.strong)))
  v(4mm)
  grid(columns: (1fr, 1fr), column-gutter: 6mm, align: top,
    {
      text(weight: "bold", L.by_section)
      v(1mm)
      table(columns: (1fr, auto, auto, auto, auto, auto), stroke: lines, fill: zebra, inset: (x: 1.6mm, y: 1.6mm), align: (x, y) => if x == 0 { left } else { center },
        table.header(..S.section_head.map(th)),
        ..S.sections.map(r => r.cells).flatten())
    },
    {
      text(weight: "bold", L.grade_distribution)
      v(1mm)
      table(columns: (auto, auto, auto, 1fr), stroke: lines, fill: zebra, inset: (x: 1.6mm, y: 1.6mm), align: (x, y) => if x == 3 { left + horizon } else { center + horizon },
        table.header(th(L.grade), th(L.count), th(L.share), []),
        ..S.grades.map(g => (text(weight: "bold", g.grade), g.count, g.share,
          box(width: 100%, height: 2.6mm, { box(width: g.ratio * 100%, height: 2.6mm, fill: accent.lighten(35%), radius: 1pt) }))).flatten())
    },
  )
  v(4mm)
  text(weight: "bold", L.subject_distribution)
  v(1mm)
  table(columns: (1fr, ..S.subject_grades.map(_ => auto)), stroke: lines, fill: zebra, inset: (x: 1.6mm, y: 1.5mm),
    align: (x, y) => if x == 0 { left + horizon } else { center + horizon },
    table.header(th(L.subject), ..S.subject_grades.map(th)),
    ..S.subjects.map(r => (r.name, ..r.counts)).flatten())
}

// Admit cards (প্রবেশপত্র / Admit Card): two per A4 sheet, with a cut line
// between them. Print-friendly: thin rules and light tints.
#import "common.typ": *
#show: base
#set page(paper: "a4", margin: (x: 8mm, y: 7mm))
#set text(size: 8pt)

#let L = doc.labels
#let card-height = 135mm

#let field(k, v) = (text(size: 6.6pt, fill: muted, k), text(weight: "semibold", v))

#let timetable(rows) = {
  let head = (L.date, L.day, L.time, L.subject, L.room, L.seat)
  let cells(r) = (text(weight: "semibold", r.date), text(fill: muted, r.day), r.time, r.subject, r.room, text(weight: "bold", r.seat))
  let one(part, size) = table(
    columns: (auto, auto, auto, 1fr, auto, auto),
    stroke: 0.4pt + rule,
    inset: (x: 1.1mm, y: if rows.len() > 9 { 0.75mm } else { 1.05mm }),
    align: (x, y) => if x == 3 { left + horizon } else { center + horizon },
    fill: (x, y) => if y == 0 { tint } else { none },
    table.header(..head.map(h => text(size: 6.2pt, weight: "bold", fill: muted, h))),
    ..part.map(r => cells(r).map(c => text(size: size, c))).flatten(),
  )
  if rows.len() == 0 {
    text(fill: muted, L.no_papers)
  } else {
    one(rows, if rows.len() > 9 { 6.6pt } else { 7.4pt })
  }
}

#let card(c) = block(width: 100%, height: card-height, stroke: 0.9pt + accent, radius: 3pt, inset: (x: 4mm, y: 3.2mm), clip: true, {
  // Header: crest, school, QR.
  grid(columns: (15mm, 1fr, 17mm), column-gutter: 2.5mm, align: (left + horizon, center + horizon, right + horizon),
    crest(14mm),
    {
      text(size: 12pt, weight: "bold", doc.school.name)
      if doc.school.name_alt != "" { linebreak(); text(size: 8pt, weight: "semibold", fill: muted, doc.school.name_alt) }
      if doc.school.at("eiin", default: "") != "" { linebreak(); text(size: 6.5pt, fill: muted, doc.school.eiin) }
      if doc.school.contact != "" { linebreak(); text(size: 6.3pt, fill: muted, doc.school.contact) }
    },
    if c.qr != "" { stack(spacing: 0.5mm, image(c.qr, width: 16mm), text(size: 5pt, fill: muted, L.scan)) } else { [] },
  )
  v(1.4mm)
  block(width: 100%, fill: tint, stroke: (left: 3pt + accent, rest: 0.5pt + accent.lighten(55%)), inset: (x: 3mm, y: 1.5mm), radius: 2pt,
    grid(columns: (auto, 1fr), align: (left + horizon, right + horizon), column-gutter: 3mm,
      { text(size: 11pt, weight: "bold", fill: accent.darken(25%), doc.heading); text(size: 8.5pt, weight: "semibold", fill: muted, "  /  " + doc.heading_alt) },
      text(size: 8pt, weight: "bold", doc.exam)))
  v(1.6mm)
  // The student.
  grid(columns: (1fr, 20mm), column-gutter: 3mm, align: (left + top, right + top),
    grid(columns: (auto, 1.5fr, auto, 1fr, auto, 1fr), column-gutter: 2mm, row-gutter: 1.6mm, align: left + horizon,
      text(size: 6.6pt, fill: muted, L.name),
      grid.cell(colspan: 5, { text(weight: "bold", size: 9pt, c.name); if c.name_alt != "" { text(size: 7.4pt, fill: muted, "  " + c.name_alt) } }),
      ..field(L.student_id, c.student_id), ..field(L.class, c.class), ..field(L.section, c.section),
      ..field(L.roll, c.roll), ..field(L.room, c.room), ..field(L.seat, c.seat),
    ),
    box(width: 20mm, height: 24mm, stroke: 0.5pt + rule, radius: 2pt, clip: true, fill: tint,
      if c.photo != "" { image(c.photo, width: 20mm, height: 24mm, fit: "cover") }
      else { align(center + horizon, text(size: 6.5pt, fill: faint, L.photo)) }),
  )
  v(1.6mm)
  text(size: 6.8pt, weight: "bold", fill: muted, L.timetable)
  v(0.4mm)
  timetable(c.rows)
  v(1fr)
  // Rules, and the head teacher's signature.
  grid(columns: (1fr, 42mm), column-gutter: 5mm, align: (left + bottom, center + bottom),
    {
      text(size: 6.6pt, weight: "bold", fill: muted, L.rules)
      v(0.2mm)
      set par(leading: 0.42em, spacing: 0.42em)
      for (i, r) in doc.rules.enumerate() { text(size: 6.2pt, str(i + 1) + ". " + r); linebreak() }
    },
    {
      box(width: 100%, height: 9mm, if doc.school.signature != "" { align(center + bottom, image(doc.school.signature, width: 100%, height: 100%, fit: "contain")) })
      line(length: 100%, stroke: 0.6pt + muted)
      v(-1mm)
      text(size: 7pt, weight: "semibold", L.head)
      if doc.head_name != "" { linebreak(); text(size: 6.2pt, fill: muted, doc.head_name) }
    })
  v(0.8mm)
  text(size: 5.6pt, fill: faint, doc.footer + if c.code != "" { "  ·  " + c.code } else { "" })
})

#let cut() = {
  v(1.3mm)
  grid(columns: (1fr, auto, 1fr), column-gutter: 2mm, align: horizon,
    line(length: 100%, stroke: (paint: faint, thickness: 0.6pt, dash: "dashed")),
    text(size: 6pt, fill: faint, L.cut),
    line(length: 100%, stroke: (paint: faint, thickness: 0.6pt, dash: "dashed")))
  v(1.3mm)
}

#for (i, c) in doc.cards.enumerate() {
  if i > 0 and calc.rem(i, 2) == 0 { pagebreak() }
  if calc.rem(i, 2) == 1 { cut() }
  card(c)
}

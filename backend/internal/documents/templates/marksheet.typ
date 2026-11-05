// Student marksheet / report card: A4 portrait, one page per student.
#import "common.typ": *
#show: base
#set page(paper: "a4", margin: (x: 13mm, top: 11mm, bottom: 11mm), foreground: watermark)

#let L = doc.labels

#let header(st) = {
  grid(columns: (22mm, 1fr, 22mm), column-gutter: 3mm, align: (left + horizon, center + horizon, right + horizon),
    crest(20mm),
    school-lines(),
    if st.qr != "" {
      stack(spacing: 0.8mm, image(st.qr, width: 19mm), text(size: 5.5pt, fill: muted, L.scan))
    } else { [] },
  )
  v(2.5mm)
  provisional-banner()
  // Title band: tinted, with an accent edge, so it prints well in grey.
  block(width: 100%, fill: tint, stroke: (left: 3pt + accent, rest: 0.5pt + accent.lighten(55%)), inset: (x: 4mm, y: 2.4mm), radius: 2pt,
    grid(columns: (1fr, auto), align: (left + horizon, right + horizon), column-gutter: 3mm,
      text(size: 11.5pt, weight: "bold", tracking: 0.4pt, fill: accent.darken(25%), doc.heading),
      text(size: 9pt, weight: "semibold", doc.exam_name)))
}

#let student-block(st) = {
  let field(k, v) = (text(size: 7.5pt, fill: muted, k), text(weight: "semibold", v))
  grid(columns: (1fr, 25mm), column-gutter: 4mm, align: (left + top, right + top),
    block(width: 100%, stroke: 0.5pt + rule, radius: 2pt, inset: (x: 3mm, y: 2.6mm),
      grid(columns: (auto, 1fr, auto, 1fr), column-gutter: 2.5mm, row-gutter: 2.4mm, align: left + horizon,
        ..field(L.name, st.name), ..field(L.student_id, st.student_id),
        ..if st.name_alt != "" { field("", st.name_alt) } else { field("", "") }, ..field(L.session, st.session),
        ..field(L.class, st.class), ..field(L.section, st.section),
        ..field(L.roll, st.roll), ..field(L.exam, st.exam),
      )),
    box(width: 25mm, height: 30mm, stroke: 0.5pt + rule, radius: 2pt, clip: true, fill: tint,
      if st.photo != "" { image(st.photo, width: 25mm, height: 30mm, fit: "cover") }
      else { align(center + horizon, text(size: 7pt, fill: faint, L.photo)) }),
  )
}

#let subject-table(st) = {
  let cols = doc.columns
  let head = (L.code, L.subject, ..cols, ..if doc.show_ct { (L.ct,) } else { () }, L.total, L.highest, L.grade, L.gp)
  let widths = (auto, 1fr, ..cols.map(_ => 14mm), ..if doc.show_ct { (14mm,) } else { () }, 19mm, 13mm, 11mm, 11mm)
  table(
    columns: widths,
    stroke: (x, y) => (bottom: if y == 0 { 0.8pt + accent.lighten(30%) } else { 0.4pt + rule }),
    inset: (x: 1.4mm, y: 1.75mm),
    align: (x, y) => if x == 1 { left + horizon } else { center + horizon },
    fill: (x, y) => if y == 0 { tint } else if calc.even(y) { rgb("#f8fafc") } else { none },
    table.header(..head.map(h => text(size: 7pt, weight: "bold", fill: muted, h))),
    ..st.rows.map(r => (
      text(fill: muted, r.code),
      {
        r.subject
        if r.optional { h(1.2mm); pill(L.fourth) }
        if r.note != "" { linebreak(); text(size: 6.5pt, fill: muted, r.note) }
      },
      ..r.parts,
      ..if doc.show_ct { (r.ct,) } else { () },
      { text(weight: "bold", r.total); if r.full != "" { text(size: 6.5pt, fill: muted, " / " + r.full) } },
      text(fill: muted, r.highest),
      text(weight: "bold", fill: if r.failed { fail-red } else { ink }, r.grade),
      r.gp,
    )).flatten(),
  )
}

#let summary(st) = {
  let tile(k, v, strong: false, tone: ink) = block(width: 100%, inset: (x: 2.6mm, y: 2.2mm), radius: 2pt,
    stroke: if strong { 1pt + accent } else { 0.5pt + rule }, fill: if strong { tint } else { none },
    { text(size: 7pt, fill: muted, k); linebreak(); text(size: if strong { 13pt } else { 10.5pt }, weight: "bold", fill: tone, v) })
  grid(columns: (1fr,) * 4, column-gutter: 2.2mm, row-gutter: 2.2mm,
    tile(L.total_marks, st.total), tile(L.gpa, st.gpa, strong: true), tile(L.gpa_without, st.gpa_without), tile(L.letter, st.letter),
    tile(L.result, st.result, strong: true, tone: if st.failed { fail-red } else { ink }), tile(L.class_pos, st.class_pos), tile(L.section_pos, st.section_pos), tile(L.published, st.published),
  )
}

#let grading() = if doc.grading.len() > 0 {
  text(size: 7pt, weight: "bold", fill: muted, L.grading)
  v(0.8mm)
  table(columns: (auto,) * 3, stroke: 0.4pt + rule, inset: (x: 1.6mm, y: 0.9mm), align: center + horizon,
    ..(L.marks_range, L.grade, L.point).map(h => text(size: 6.3pt, weight: "bold", fill: muted, h)),
    ..doc.grading.map(g => (text(size: 6.3pt, g.range), text(size: 6.3pt, weight: "semibold", g.grade), text(size: 6.3pt, g.point))).flatten())
}

#let signatures() = {
  let slot(title, image-name: "", name: "") = align(center + bottom, {
    if image-name != "" { image(image-name, height: 11mm, fit: "contain") } else { v(11mm) }
    line(length: 100%, stroke: 0.6pt + muted)
    v(-1mm)
    text(size: 7.5pt, weight: "semibold", title)
    // Every slot keeps a second line, so the signature lines align.
    linebreak()
    text(size: 6.5pt, fill: muted, if name != "" { name } else { sym.zws })
  })
  grid(columns: (1fr, 1fr, 1fr), column-gutter: 8mm,
    slot(L.sig_class_teacher), slot(L.sig_head, image-name: doc.school.signature, name: doc.school.signatory),
    slot(L.sig_guardian))
}

#let card(st) = {
  header(st)
  v(3mm)
  student-block(st)
  v(3.2mm)
  if st.withheld {
    block(width: 100%, inset: 6mm, radius: 3pt, stroke: 0.8pt + accent, fill: tint,
      align(center, { text(size: 12pt, weight: "bold", L.withheld); linebreak(); text(fill: muted, L.withheld_note) }))
  } else {
    subject-table(st)
    v(3mm)
    summary(st)
  }
  v(1fr)
  grid(columns: (auto, 1fr), column-gutter: 8mm, align: (left + bottom, left + bottom), grading(), signatures())
  v(2.5mm)
  line(length: 100%, stroke: 0.4pt + rule)
  v(-0.6mm)
  text(size: 6.5pt, fill: muted, st.footer)
}

#for (i, st) in doc.students.enumerate() {
  if i > 0 { pagebreak() }
  card(st)
}

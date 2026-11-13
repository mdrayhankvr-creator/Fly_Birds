// Transcript v3 (ফলাফল পত্র / Academic Transcript): A4 portrait, one page per
// student. The layout follows a board-style transcript: a decorative hatched
// border, the school's crest and name, an "Academic Transcript" badge, the
// student's details beside the grade reference, one marks table whose
// component columns come from the blueprint, then positions, attendance,
// the class teacher's comment and the signature boxes.
#import "common.typ": *
#show: base

#let L = doc.labels

// A hatched band: diagonal rules clipped to the page, with the middle
// covered again so only the frame shows.
#let hatch(w, h, step: 2.4mm) = box(width: w, height: h, clip: true, {
  let n = calc.ceil((w + h) / step)
  for i in range(n) {
    place(top + left, dx: i * step - h, line(start: (0mm, 0mm), end: (h, h), stroke: 1.1pt + accent))
  }
})

#let page-decor = {
  if doc.school.at("border", default: false) {
    place(top + left, hatch(210mm, 297mm))
    place(top + left, dx: 6.5mm, dy: 6.5mm, rect(width: 197mm, height: 284mm, fill: white, stroke: none))
    place(top + left, dx: 9mm, dy: 9mm, rect(width: 192mm, height: 279mm, fill: none, stroke: 0.7pt + accent))
  }
  // The school's own mark, washed out behind the page.
  if doc.school.at("watermark", default: false) and doc.school.logo != "" {
    place(center + horizon, box(width: 95mm, height: 95mm, {
      image(doc.school.logo, width: 100%, height: 100%, fit: "contain")
      place(top + left, rect(width: 100%, height: 100%, fill: white.transparentize(6%)))
    }))
  }
}

#set page(paper: "a4", margin: (x: 14mm, top: 13mm, bottom: 12mm), background: page-decor, foreground: watermark)
#set text(size: 8.6pt)

// ---------------------------------------------------------------------------
// Header: crest, school name, "Established-<year>", the transcript badge.

#let header(st) = {
  grid(columns: (24mm, 1fr, 24mm), column-gutter: 3mm, align: (left + horizon, center + horizon, right + horizon),
    crest(22mm),
    {
      text(size: 16pt, weight: "bold", tracking: 0.2pt, doc.school.name)
      if doc.school.name_alt != "" { linebreak(); text(size: 10.5pt, weight: "semibold", fill: muted, doc.school.name_alt) }
      if doc.school.at("established", default: "") != "" { linebreak(); text(size: 10pt, weight: "bold", doc.school.established) }
      if doc.school.at("eiin", default: "") != "" or doc.school.contact != "" {
        linebreak()
        text(size: 7pt, fill: muted, (doc.school.at("eiin", default: ""), doc.school.contact).filter(x => x != "").join("  ·  "))
      }
    },
    if st.photo != "" {
      box(width: 21mm, height: 25mm, stroke: 0.5pt + rule, radius: 2pt, clip: true, image(st.photo, width: 21mm, height: 25mm, fit: "cover"))
    } else { [] },
  )
  v(2.4mm)
  provisional-banner()
  align(center, block(fill: accent, radius: 3pt, inset: (x: 6mm, y: 2mm),
    text(size: 11pt, weight: "bold", fill: white, doc.heading + " — " + doc.exam_name)))
}

// ---------------------------------------------------------------------------
// Student information (left) and the grade reference (right).

#let info-table(st) = {
  // Only the details this school and class record; the pairs fill the two
  // halves of the table in order.
  let pairs = ((L.student_id, st.student_id), (L.group, st.at("group", default: "")),
    (L.name, st.name), (L.class, st.class),
    (L.session, st.session), (L.section, st.section),
    (L.version_medium, st.at("medium", default: "")), (L.roll, st.roll),
    (L.shift, st.at("shift", default: "")), (L.fourth_short, st.at("fourth", default: "")))
  let shown = pairs.filter(p => p.at(1) != "" and p.at(1) != "—")
  let strong-value = (L.student_id, L.name)
  table(columns: (auto, 1fr, auto, 1fr), stroke: 0.5pt + rule, inset: (x: 2.2mm, y: 1.5mm),
    align: left + horizon, fill: (x, y) => if y == 0 { tint } else { none },
    table.cell(colspan: 4, align: center, text(size: 9pt, weight: "bold", L.student_information)),
    ..shown.map(p => (
      text(size: 7.6pt, fill: muted, p.at(0)),
      text(size: 8.4pt, weight: if strong-value.contains(p.at(0)) { "bold" } else { "regular" }, p.at(1)),
    )).flatten(),
    // An odd number of details leaves the last half-row blank.
    ..if calc.odd(shown.len()) { ([], []) } else { () },
  )
}

#let grading() = if doc.grading.len() > 0 {
  table(columns: (auto, auto, auto), stroke: 0.5pt + rule, inset: (x: 1.8mm, y: 1.05mm), align: center + horizon,
    fill: (x, y) => if y <= 1 { tint } else { none },
    table.cell(colspan: 3, text(size: 8pt, weight: "bold", L.grades_reference)),
    ..(L.grade, L.point, L.marks_range).map(h => text(size: 7pt, weight: "bold", h)),
    ..doc.grading.map(g => (text(size: 7pt, g.grade), text(size: 7pt, g.point), text(size: 7pt, g.range + "%"))).flatten())
}

// ---------------------------------------------------------------------------
// The marks table. Columns: subject, full marks, one per component the
// blueprint uses (under "Obtained Marks"), total, highest, GP and the letter
// grade. A component a subject does not have prints "—"; one below its own
// pass marks is red.

#let mark(c) = {
  let failed = c.at("failed", default: false)
  text(fill: if failed { fail-red } else { ink }, weight: if failed { "semibold" } else { "regular" }, c.obt)
  if c.full != "" { text(size: 6.2pt, fill: muted, "/" + c.full) }
}

#let subject-table(st) = {
  let cols = doc.columns
  let parts = (if doc.show_ct { (L.ct,) } else { () }) + cols
  let widths = (1fr, 13mm, ..parts.map(_ => 15mm), 13mm, 14mm, 11mm, 10mm)
  let h(body) = text(size: 7.2pt, weight: "bold", body)
  let span(body) = table.cell(rowspan: 2, h(body))
  // One row per paper of a combined subject; the shared cells span them.
  let emit(r, shade) = {
    let papers = if r.papers.len() > 1 { r.papers } else { ((code: r.code, subject: r.subject, parts: r.parts),) }
    let n = papers.len()
    let cell(body, rows: 1) = table.cell(rowspan: rows, fill: shade, body)
    let out = ()
    for (i, p) in papers.enumerate() {
      out.push(table.cell(align: left + horizon, fill: shade, {
        p.subject
        if r.optional and i == 0 { " " + L.fourth_suffix }
        if r.note != "" and i == n - 1 { linebreak(); text(size: 6.3pt, fill: muted, r.note) }
      }))
      if i == 0 { out.push(cell(if r.full != "" { r.full } else { "—" }, rows: n)) }
      if i == 0 and doc.show_ct { out.push(cell(r.ct, rows: n)) }
      for c in p.parts { out.push(table.cell(fill: shade, mark(c))) }
      if i == 0 {
        out.push(cell(text(weight: "bold", r.total), rows: n))
        out.push(cell(text(fill: muted, r.highest), rows: n))
        out.push(cell(r.gp, rows: n))
        out.push(cell(text(weight: "bold", fill: if r.failed { fail-red } else { ink }, r.grade), rows: n))
      }
    }
    out
  }
  table(
    columns: widths,
    stroke: 0.5pt + rule,
    inset: (x: 1.6mm, y: 1.5mm),
    align: center + horizon,
    table.header(
      table.cell(rowspan: 2, align: left + horizon, fill: tint, h(L.subject)),
      table.cell(rowspan: 2, fill: tint, h(L.full_marks)),
      table.cell(colspan: parts.len(), fill: tint, h(L.obtained_marks)),
      table.cell(rowspan: 2, fill: tint, h(L.total)),
      table.cell(rowspan: 2, fill: tint, h(L.highest)),
      table.cell(rowspan: 2, fill: tint, h(L.gp)),
      table.cell(rowspan: 2, fill: tint, h(L.lg)),
      ..parts.map(p => table.cell(fill: tint, h(p))),
    ),
    ..st.rows.map(r => emit(r, none)).flatten(),
    // The 4th subject keeps its own lightly shaded row.
    ..st.optional.map(r => emit(r, rgb("#fdf6e3"))).flatten(),
    // Total / GPA.
    table.cell(align: left + horizon, fill: tint.lighten(35%), text(weight: "bold", L.total_gpa)),
    table.cell(fill: tint.lighten(35%), text(weight: "bold", st.full_marks)),
    ..parts.map(_ => table.cell(fill: tint.lighten(35%), [])),
    table.cell(fill: tint.lighten(35%), text(weight: "bold", st.total_marks)),
    table.cell(fill: tint.lighten(35%), []),
    table.cell(fill: tint.lighten(35%), text(weight: "bold", st.gpa)),
    table.cell(fill: tint.lighten(35%), text(weight: "bold", fill: if st.failed { fail-red } else { ink }, st.letter)),
  )
}

// ---------------------------------------------------------------------------
// Result, positions and attendance (left); comment and signatures (right).

#let standing(st) = {
  let row(k, v, strong: false) = (
    table.cell(align: left + horizon, fill: if strong { tint } else { none }, text(size: 8pt, weight: if strong { "bold" } else { "regular" }, k)),
    table.cell(fill: if strong { tint } else { none }, text(size: 8.6pt, weight: "bold", v)),
  )
  table(columns: (auto, 1fr), stroke: 0.5pt + rule, inset: (x: 2.2mm, y: 1.5mm), align: center + horizon,
    ..row(L.result, st.result, strong: true),
    ..row(L.section_pos, st.section_pos, strong: true),
    ..row(L.class_pos, st.class_pos, strong: true),
    ..row(L.classes_held, st.held),
    ..row(L.days_present, st.present_days),
    ..row(L.days_absent, st.absent_days),
  )
}

#let comments(st) = {
  table(columns: (auto, 1fr), stroke: 0.5pt + rule, inset: (x: 2.5mm, y: 2mm), align: left + horizon,
    text(size: 8pt, weight: "bold", L.comment),
    if st.comment != "" { text(size: 8.4pt, st.comment) } else { box(width: 100%, height: 4.5mm) })
  v(2.6mm)
  // The signature boxes: the class teacher's and the head teacher's images
  // are placed automatically; the guardian signs by hand.
  let slot(sig) = table.cell(align: center + horizon, {
    text(size: 7.8pt, weight: "bold", sig.title)
    linebreak()
    text(size: 7.8pt, weight: "bold", L.signature)
  })
  table(columns: (1fr,) * st.signatures.len(), stroke: 0.5pt + rule, inset: (x: 2mm, y: 1.8mm),
    ..st.signatures.map(slot),
    ..st.signatures.map(sig => table.cell(align: center + bottom, box(width: 100%, height: 14mm,
      if sig.image != "" { align(center + bottom, image(sig.image, width: 100%, height: 100%, fit: "contain")) }
      else if sig.date { align(center + bottom, text(size: 6.6pt, fill: muted, L.sig_date + ": ____ / ____ / ________")) }
      else { [] }))),
  )
}

// ---------------------------------------------------------------------------

#let card(st) = {
  header(st)
  v(3.4mm)
  if st.withheld {
    block(width: 100%, inset: 6mm, radius: 3pt, stroke: 0.8pt + accent, fill: tint,
      align(center, { text(size: 12pt, weight: "bold", L.withheld); linebreak(); text(fill: muted, L.withheld_note) }))
  } else {
    grid(columns: (1fr, 42mm), column-gutter: 5mm, align: (left + top, right + top), info-table(st), grading())
    v(3.4mm)
    subject-table(st)
    v(1.4mm)
    text(size: 6.4pt, fill: muted, L.transcript_note)
    if st.gpa_without != "—" { text(size: 6.4pt, fill: muted, "  " + L.gpa_without + ": " + st.gpa_without + ".") }
    v(3.4mm)
    grid(columns: (58mm, 1fr), column-gutter: 5mm, align: (left + top, left + top), standing(st), comments(st))
  }
  v(1fr)
  grid(columns: (auto, 1fr), column-gutter: 3mm, align: (left + horizon, left + horizon),
    if st.qr != "" { stack(spacing: 0.6mm, image(st.qr, width: 15mm), text(size: 5pt, fill: muted, L.scan)) } else { [] },
    { line(length: 100%, stroke: 0.4pt + rule); v(-0.6mm); text(size: 6.2pt, fill: muted, st.footer) })
}

#for (i, st) in doc.students.enumerate() {
  if i > 0 { pagebreak() }
  card(st)
}

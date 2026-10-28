from pathlib import Path

from docx import Document
from docx.enum.section import WD_ORIENT
from docx.enum.table import WD_ALIGN_VERTICAL, WD_CELL_VERTICAL_ALIGNMENT
from docx.enum.text import WD_ALIGN_PARAGRAPH
from docx.oxml import OxmlElement
from docx.oxml.ns import qn
from docx.shared import Cm, Inches, Pt, RGBColor


OUTPUT_DIR = Path("word_output")
OUTPUT_DIR.mkdir(exist_ok=True)

BLACK = "111111"
GRAY = "6B7280"
LIGHT = "F3F4F6"
GRID = "D9D9D9"
RED = "E84B4B"
PURPLE = "A63B8E"
ORANGE = "E57A34"
GREEN = "2D8C72"
YELLOW = "F4D35E"


def set_cell_fill(cell, color):
    tc_pr = cell._tc.get_or_add_tcPr()
    shd = tc_pr.find(qn("w:shd"))
    if shd is None:
        shd = OxmlElement("w:shd")
        tc_pr.append(shd)
    shd.set(qn("w:fill"), color)


def set_cell_borders(cell, color=GRID, size="4"):
    tc_pr = cell._tc.get_or_add_tcPr()
    borders = tc_pr.first_child_found_in("w:tcBorders")
    if borders is None:
        borders = OxmlElement("w:tcBorders")
        tc_pr.append(borders)
    for edge in ("top", "left", "bottom", "right", "insideH", "insideV"):
        tag = "w:" + edge
        element = borders.find(qn(tag))
        if element is None:
            element = OxmlElement(tag)
            borders.append(element)
        element.set(qn("w:val"), "single")
        element.set(qn("w:sz"), size)
        element.set(qn("w:color"), color)


def set_cell_margins(cell, top=65, start=75, bottom=65, end=75):
    tc = cell._tc
    tc_pr = tc.get_or_add_tcPr()
    tc_mar = tc_pr.first_child_found_in("w:tcMar")
    if tc_mar is None:
        tc_mar = OxmlElement("w:tcMar")
        tc_pr.append(tc_mar)
    for m, value in (("top", top), ("start", start), ("bottom", bottom), ("end", end)):
        node = tc_mar.find(qn("w:" + m))
        if node is None:
            node = OxmlElement("w:" + m)
            tc_mar.append(node)
        node.set(qn("w:w"), str(value))
        node.set(qn("w:type"), "dxa")


def set_table_widths(table, widths_cm):
    for column, width in zip(table.columns, widths_cm):
        column.width = Cm(width)
    for row in table.rows:
        for cell, width in zip(row.cells, widths_cm):
            cell.width = Cm(width)


def format_run(run, size=9, bold=False, color=BLACK, font="Arial"):
    run.font.name = font
    run._element.get_or_add_rPr().get_or_add_rFonts().set(qn("w:ascii"), font)
    run._element.get_or_add_rPr().get_or_add_rFonts().set(qn("w:hAnsi"), font)
    run._element.get_or_add_rPr().get_or_add_rFonts().set(qn("w:eastAsia"), font)
    run.font.size = Pt(size)
    run.font.bold = bold
    run.font.color.rgb = RGBColor.from_string(color)
    return run


def clear_para(paragraph):
    for child in list(paragraph._p):
        paragraph._p.remove(child)


def write_cell(cell, text, size=8.5, bold=False, color=BLACK,
               align=WD_ALIGN_PARAGRAPH.LEFT, fill=None):
    cell.vertical_alignment = WD_CELL_VERTICAL_ALIGNMENT.CENTER
    if fill:
        set_cell_fill(cell, fill)
    set_cell_borders(cell)
    set_cell_margins(cell)
    p = cell.paragraphs[0]
    clear_para(p)
    p.alignment = align
    p.paragraph_format.space_before = Pt(0)
    p.paragraph_format.space_after = Pt(0)
    p.paragraph_format.line_spacing = 1.0
    format_run(p.add_run(str(text)), size=size, bold=bold, color=color)


def add_label_value(cell, label, value, accent=RED, value_size=18):
    set_cell_borders(cell)
    set_cell_margins(cell, top=90, start=110, bottom=90, end=110)
    p = cell.paragraphs[0]
    clear_para(p)
    p.paragraph_format.space_after = Pt(2)
    format_run(p.add_run(label.upper()), size=7.5, bold=True, color=GRAY)
    p2 = cell.add_paragraph()
    p2.paragraph_format.space_before = Pt(0)
    p2.paragraph_format.space_after = Pt(0)
    format_run(p2.add_run(value), size=value_size, bold=True, color=accent)


def style_document(doc):
    section = doc.sections[0]
    section.orientation = WD_ORIENT.LANDSCAPE
    section.page_width = Cm(29.7)
    section.page_height = Cm(21.0)
    section.top_margin = Cm(0.55)
    section.bottom_margin = Cm(0.55)
    section.left_margin = Cm(0.65)
    section.right_margin = Cm(0.65)
    section.header_distance = Cm(0.2)
    section.footer_distance = Cm(0.2)

    normal = doc.styles["Normal"]
    normal.font.name = "Arial"
    normal._element.rPr.rFonts.set(qn("w:ascii"), "Arial")
    normal._element.rPr.rFonts.set(qn("w:hAnsi"), "Arial")
    normal.font.size = Pt(9)
    normal.font.color.rgb = RGBColor.from_string(BLACK)
    normal.paragraph_format.space_after = Pt(0)

    title = doc.styles["Title"]
    title.font.name = "Arial"
    title._element.rPr.rFonts.set(qn("w:ascii"), "Arial")
    title._element.rPr.rFonts.set(qn("w:hAnsi"), "Arial")
    title.font.size = Pt(15)
    title.font.bold = True
    title.font.color.rgb = RGBColor.from_string(BLACK)


def add_title(doc, process_word):
    p = doc.add_paragraph(style="Title")
    p.paragraph_format.space_before = Pt(0)
    p.paragraph_format.space_after = Pt(4)
    p.paragraph_format.line_spacing = 0.95
    format_run(p.add_run(f"Карта {process_word} процесса сбора и вывоза\n"), size=15, bold=True)
    format_run(p.add_run("твёрдых коммунальных отходов"), size=15, bold=True, color=RED)
    format_run(
        p.add_run(" Горняцкого внутригородского района администрации г.о. Макеевка"),
        size=15,
        bold=True,
    )


def add_top_block(doc, data):
    table = doc.add_table(rows=1, cols=2)
    table.autofit = False
    set_table_widths(table, [18.1, 9.7])
    left, right = table.rows[0].cells
    for cell in (left, right):
        set_cell_borders(cell)
        set_cell_margins(cell, top=80, start=100, bottom=80, end=100)

    p = left.paragraphs[0]
    p.paragraph_format.space_after = Pt(0)
    format_run(p.add_run(data["expense_label"].upper()), size=7.5, bold=True, color=GRAY)
    p2 = left.add_paragraph()
    p2.paragraph_format.space_after = Pt(0)
    format_run(p2.add_run(data["monthly"]), size=31, bold=True)
    p3 = left.add_paragraph()
    p3.paragraph_format.space_after = Pt(4)
    format_run(p3.add_run("в месяц — среднемесячные расходы на работу мусоровоза"), size=7.5, color=GRAY)

    bar = left.add_table(rows=1, cols=2)
    bar.autofit = False
    set_table_widths(bar, [12.8, 4.6])
    write_cell(bar.cell(0, 0), f"Эксплуатация · {data['operation']}", size=8.5, bold=True, color="FFFFFF", fill="3B3B3B")
    write_cell(bar.cell(0, 1), f"{data['service_label']} · {data['service']}", size=8.3, bold=True, color="FFFFFF", fill=RED)

    shares = left.add_table(rows=1, cols=2)
    shares.autofit = False
    set_table_widths(shares, [12.8, 4.6])
    write_cell(shares.cell(0, 0), data["operation_share"], size=7.5, color=GRAY)
    write_cell(shares.cell(0, 1), data["service_share"], size=7.5, color=RED)

    metrics = left.add_table(rows=1, cols=3)
    metrics.autofit = False
    set_table_widths(metrics, [6.0, 6.0, 5.4])
    for i, (value, label) in enumerate(data["metrics"]):
        cell = metrics.cell(0, i)
        set_cell_borders(cell)
        set_cell_margins(cell, top=50, start=70, bottom=50, end=70)
        pp = cell.paragraphs[0]
        clear_para(pp)
        pp.alignment = WD_ALIGN_PARAGRAPH.CENTER
        format_run(pp.add_run(value), size=15, bold=True)
        pp2 = cell.add_paragraph()
        pp2.alignment = WD_ALIGN_PARAGRAPH.CENTER
        format_run(pp2.add_run(label), size=6.5, color=GRAY)

    rp = right.paragraphs[0]
    rp.paragraph_format.space_after = Pt(2)
    format_run(rp.add_run("ТЕХНИКА"), size=7.5, bold=True, color=GRAY)
    rp2 = right.add_paragraph()
    rp2.paragraph_format.space_after = Pt(0)
    format_run(rp2.add_run(data["vehicle"]), size=18, bold=True)
    if data.get("plate"):
        rp3 = right.add_paragraph()
        rp3.paragraph_format.space_after = Pt(4)
        format_run(rp3.add_run(data["plate"]), size=10, bold=True, color=GRAY)
    rp4 = right.add_paragraph()
    rp4.paragraph_format.space_after = Pt(4)
    format_run(rp4.add_run("мусоровоз, задействованный в вывозе ТКО"), size=7, color=GRAY)

    specs = right.add_table(rows=3, cols=2)
    specs.autofit = False
    set_table_widths(specs, [4.0, 5.2])
    for row, (label, value) in enumerate((("Год выпуска", data["year"]), ("Возраст", data["age"]), ("Износ", data["wear"]))):
        write_cell(specs.cell(row, 0), label, size=7.5, color=GRAY)
        write_cell(specs.cell(row, 1), value, size=10 if row < 2 else 16, bold=True, color=RED if row == 2 else BLACK, align=WD_ALIGN_PARAGRAPH.RIGHT)

    annual = right.add_table(rows=1, cols=1)
    add_label_value(annual.cell(0, 0), data["annual_label"], data["annual"], accent=RED, value_size=14)


def add_process_block(doc, data):
    p = doc.add_paragraph()
    p.paragraph_format.space_before = Pt(4)
    p.paragraph_format.space_after = Pt(2)
    format_run(p.add_run(data["scheme_title"].upper()), size=8, bold=True, color=GRAY)
    if data.get("scheme_note"):
        format_run(p.add_run("  " + data["scheme_note"]), size=8, color=GRAY)

    routes = doc.add_table(rows=4, cols=5)
    routes.autofit = False
    set_table_widths(routes, [3.6, 4.0, 10.0, 4.0, 6.0])
    garage = routes.cell(0, 0).merge(routes.cell(3, 0))
    landfill = routes.cell(0, 4).merge(routes.cell(3, 4))
    write_cell(garage, "ГАРАЖ\nвыезд мусоровоза", size=8, bold=True, fill=LIGHT, align=WD_ALIGN_PARAGRAPH.CENTER)
    write_cell(landfill, "ПОЛИГОН ТКО\nотвалы шахты им. Бажанова", size=8, bold=True, fill=LIGHT, align=WD_ALIGN_PARAGRAPH.CENTER)

    for i, route in enumerate(data["routes"]):
        write_cell(routes.cell(i, 1), f"→  {route['outbound']}", size=8, bold=True, color="FFFFFF", fill=PURPLE, align=WD_ALIGN_PARAGRAPH.CENTER)
        write_cell(
            routes.cell(i, 2),
            f"{route['name']}\n{route['sites']} площадок · {route['containers']} контейнеров · {route['capacity']}",
            size=7.7,
            bold=True,
            fill="FFFFFF" if i % 2 == 0 else "FAFAFA",
        )
        write_cell(routes.cell(i, 3), f"{route['landfill']}  →", size=8, bold=True, color="FFFFFF", fill=ORANGE, align=WD_ALIGN_PARAGRAPH.CENTER)


def add_summary(doc, data):
    p = doc.add_paragraph()
    p.paragraph_format.space_before = Pt(4)
    p.paragraph_format.space_after = Pt(2)
    format_run(p.add_run("СВОДНЫЕ ПОКАЗАТЕЛИ"), size=8, bold=True, color=GRAY)

    headers = ["Показатель", "Всего", "Холодная Балка", "Новая Заря", "Ломбардо", "Межевое"]
    rows = [
        ("Количество потребителей МКД, чел.", ["2 334", "1 251", "203", "155", "725"]),
        ("Объём образования ТКО, м³/сут", ["87", "50", "10", "15", "12"]),
        ("Количество контейнерных площадок, шт.", ["20", "10", "3", "5", "2"]),
        (data["container_row_label"], data["container_totals"]),
    ]
    table = doc.add_table(rows=1 + len(rows), cols=6)
    table.autofit = False
    set_table_widths(table, [7.3, 2.5, 4.8, 4.2, 4.2, 4.2])
    for i, head in enumerate(headers):
        fill = "404040" if i < 2 else ("DDEBF7" if i == 2 else "F3F4F6")
        color = "FFFFFF" if i < 2 else BLACK
        write_cell(table.cell(0, i), head, size=7.3, bold=True, color=color, fill=fill, align=WD_ALIGN_PARAGRAPH.CENTER)
    for r, (label, values) in enumerate(rows, start=1):
        write_cell(table.cell(r, 0), label, size=7.1, color=GRAY)
        for c, value in enumerate(values, start=1):
            write_cell(table.cell(r, c), value, size=7.8, bold=(c == 1), color=RED if c == 1 else BLACK, align=WD_ALIGN_PARAGRAPH.CENTER)


def create_map(path, data):
    doc = Document()
    style_document(doc)
    add_title(doc, data["process_word"])
    add_top_block(doc, data)
    add_process_block(doc, data)
    add_summary(doc, data)

    props = doc.core_properties
    props.title = f"Карта {data['process_word']} процесса сбора и вывоза ТКО"
    props.subject = "Редактируемая карта процесса"
    props.author = ""
    props.keywords = "ТКО, карта процесса, Макеевка"
    doc.save(path)


common_routes = [
    ("Холодная Балка", "13,3 км", "17,6 км", "10"),
    ("Новая Заря", "12,4 км", "16,8 км", "3"),
    ("Ломбардо", "16,2 км", "20,0 км", "5"),
    ("Межевое", "17,8 км", "22,9 км", "2"),
]

target_counts = ["60", "9", "9", "5"]
current_counts = ["86", "13", "12", "9"]

target = {
    "process_word": "целевого",
    "expense_label": "Расходы на вывоз ТКО после оптимизации процесса",
    "monthly": "346 345 ₽",
    "operation": "384 195 ₽",
    "service_label": "Обслуживание",
    "service": "8 333 ₽",
    "operation_share": "84,5 % — эксплуатация",
    "service_share": "15,5 % — обслуживание",
    "metrics": [
        ("≈ 5,45 млн ₽", "в год на работу мусоровоза"),
        ("≈ 195 ₽", "на 1 потребителя МКД в месяц"),
        ("≈ 3 787 ₽", "на 1 контейнер в месяц"),
    ],
    "vehicle": "КАМАЗ 53605",
    "plate": "КМ 7028-67",
    "year": "2025",
    "age": "1 год",
    "wear": "10 %",
    "annual_label": "Обслуживание за год",
    "annual": "≈ 842 568 ₽",
    "scheme_title": "Схема целевого процесса",
    "scheme_note": "вывоз ТКО после оптимизации процесса",
    "routes": [
        {"name": name, "outbound": out, "landfill": land, "sites": sites, "containers": count, "capacity": "1,1 м³"}
        for (name, out, land, sites), count in zip(common_routes, target_counts)
    ],
    "container_row_label": "Количество контейнеров 1,1 м³, шт.",
    "container_totals": ["83", "60", "9", "9", "5"],
}

current = {
    "process_word": "текущего",
    "expense_label": "Текущие расходы на вывоз ТКО",
    "monthly": "454 409 ₽",
    "operation": "384 195 ₽",
    "service_label": "Ремонт",
    "service": "70 214 ₽",
    "operation_share": "84,5 % — эксплуатация",
    "service_share": "15,5 % — ремонт",
    "metrics": [
        ("≈ 5,45 млн ₽", "в год на работу мусоровоза"),
        ("≈ 195 ₽", "на 1 потребителя МКД в месяц"),
        ("≈ 3 787 ₽", "на 1 контейнер в месяц"),
    ],
    "vehicle": "DAF 75CF50",
    "plate": "",
    "year": "2009",
    "age": "17 лет",
    "wear": "80 %",
    "annual_label": "Ремонт за год",
    "annual": "≈ 842 568 ₽",
    "scheme_title": "Схема текущего процесса",
    "scheme_note": "",
    "routes": [
        {"name": name, "outbound": out, "landfill": land, "sites": sites, "containers": count, "capacity": "0,75 м³"}
        for (name, out, land, sites), count in zip(common_routes, current_counts)
    ],
    "container_row_label": "Количество контейнеров 0,75 м³, шт.",
    "container_totals": ["120", "86", "13", "12", "9"],
}


create_map(OUTPUT_DIR / "karta_celevogo_processa.docx", target)
create_map(OUTPUT_DIR / "karta_tekushchego_processa.docx", current)
print(OUTPUT_DIR / "karta_celevogo_processa.docx")
print(OUTPUT_DIR / "karta_tekushchego_processa.docx")

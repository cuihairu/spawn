// 极简 Markdown 解析：不引第三方依赖（RN 无 DOM，web 的 marked/DOMPurify
// 方案不适用），按行扫描出块级结构，块内做行内 token 切分。
// 支持子集：标题 #/##/###、段落、围栏代码块、无序/有序列表、引用、分割线、
// 行内 **粗体** / *斜体* / `代码` / [链接](url)。

export type InlineSegment =
  | { type: 'text'; text: string }
  | { type: 'bold'; text: string }
  | { type: 'italic'; text: string }
  | { type: 'code'; text: string }
  | { type: 'link'; text: string; href: string };

export type Block =
  | { type: 'heading'; level: 1 | 2 | 3; segments: InlineSegment[] }
  | { type: 'paragraph'; segments: InlineSegment[] }
  | { type: 'codeBlock'; text: string }
  | { type: 'list'; ordered: boolean; items: InlineSegment[][] }
  | { type: 'quote'; segments: InlineSegment[] }
  | { type: 'hr' };

const HEADING_RE = /^(#{1,3})\s+(.*)$/;
const UNORDERED_ITEM_RE = /^[-*+]\s+(.*)$/;
const ORDERED_ITEM_RE = /^\d+[.)]\s+(.*)$/;
const LINK_RE = /^\[([^\]]*)\]\(([^()\s]+)\)/;

function parseInline(text: string): InlineSegment[] {
  const segments: InlineSegment[] = [];
  let buffer = '';
  let i = 0;

  const flush = () => {
    if (buffer) {
      segments.push({ type: 'text', text: buffer });
      buffer = '';
    }
  };

  while (i < text.length) {
    // **粗体**（先于单字符斜体判断）
    if (text.startsWith('**', i)) {
      const end = text.indexOf('**', i + 2);
      if (end > i + 2) {
        flush();
        segments.push({ type: 'bold', text: text.slice(i + 2, end) });
        i = end + 2;
        continue;
      }
    }
    // `行内代码`
    if (text[i] === '`') {
      const end = text.indexOf('`', i + 1);
      if (end > i + 1) {
        flush();
        segments.push({ type: 'code', text: text.slice(i + 1, end) });
        i = end + 1;
        continue;
      }
    }
    // *斜体* / _斜体_
    if (text[i] === '*' || text[i] === '_') {
      const end = text.indexOf(text[i], i + 1);
      if (end > i + 1) {
        flush();
        segments.push({ type: 'italic', text: text.slice(i + 1, end) });
        i = end + 1;
        continue;
      }
    }
    // [链接](url)
    if (text[i] === '[') {
      const match = LINK_RE.exec(text.slice(i));
      if (match) {
        flush();
        segments.push({ type: 'link', text: match[1], href: match[2] });
        i += match[0].length;
        continue;
      }
    }
    buffer += text[i];
    i += 1;
  }
  flush();
  return segments;
}

export function parseMarkdown(source: string): Block[] {
  const lines = source.replace(/\r\n/g, '\n').split('\n');
  const blocks: Block[] = [];
  let paragraph: string[] = [];
  let list: { ordered: boolean; items: InlineSegment[][] } | null = null;
  let quote: string[] = [];

  const endParagraph = () => {
    if (paragraph.length) {
      blocks.push({ type: 'paragraph', segments: parseInline(paragraph.join(' ')) });
      paragraph = [];
    }
  };
  const endList = () => {
    if (list) {
      blocks.push({ type: 'list', ordered: list.ordered, items: list.items });
      list = null;
    }
  };
  const endQuote = () => {
    if (quote.length) {
      blocks.push({ type: 'quote', segments: parseInline(quote.join(' ')) });
      quote = [];
    }
  };
  const endAll = () => {
    endParagraph();
    endList();
    endQuote();
  };

  let inFence = false;
  let fenceText: string[] = [];

  for (const line of lines) {
    // 围栏代码块优先：``` 开启/关闭，内部内容原样保留
    if (line.trimStart().startsWith('```')) {
      if (inFence) {
        blocks.push({ type: 'codeBlock', text: fenceText.join('\n') });
        fenceText = [];
        inFence = false;
      } else {
        endAll();
        inFence = true;
      }
      continue;
    }
    if (inFence) {
      fenceText.push(line);
      continue;
    }

    if (!line.trim()) {
      endAll();
      continue;
    }

    const heading = HEADING_RE.exec(line);
    if (heading) {
      endAll();
      blocks.push({
        type: 'heading',
        level: heading[1].length as 1 | 2 | 3,
        segments: parseInline(heading[2]),
      });
      continue;
    }

    if (/^\s*(---+|\*{3,}|_{3,})\s*$/.test(line)) {
      endAll();
      blocks.push({ type: 'hr' });
      continue;
    }

    const unordered = UNORDERED_ITEM_RE.exec(line.trim());
    if (unordered) {
      endParagraph();
      endQuote();
      if (!list || list.ordered) {
        endList();
        list = { ordered: false, items: [] };
      }
      list.items.push(parseInline(unordered[1]));
      continue;
    }

    const ordered = ORDERED_ITEM_RE.exec(line.trim());
    if (ordered) {
      endParagraph();
      endQuote();
      if (!list || !list.ordered) {
        endList();
        list = { ordered: true, items: [] };
      }
      list.items.push(parseInline(ordered[1]));
      continue;
    }

    if (line.trimStart().startsWith('>')) {
      endParagraph();
      endList();
      quote.push(line.trimStart().slice(1).trimStart());
      continue;
    }

    // 普通文本行：并入段落
    endList();
    endQuote();
    paragraph.push(line.trim());
  }

  if (inFence && fenceText.length) {
    blocks.push({ type: 'codeBlock', text: fenceText.join('\n') });
  }
  endAll();
  return blocks;
}

// 去掉 Markdown 标记取纯文本：摘要兜底等单行场景用。
export function stripMarkdown(source: string): string {
  return source
    .replace(/```[\s\S]*?```/g, ' ')
    .replace(/^#{1,3}\s+/gm, '')
    .replace(/^[-*+]\s+/gm, '')
    .replace(/^\d+[.)]\s+/gm, '')
    .replace(/^>\s?/gm, '')
    .replace(/^\s*(---+|\*{3,}|_{3,})\s*$/gm, '')
    .replace(/\*\*([^*]+)\*\*/g, '$1')
    .replace(/[*_`]([^*_`]+)[*_`]/g, '$1')
    .replace(/\[([^\]]*)\]\(([^()\s]+)\)/g, '$1')
    .replace(/\s+/g, ' ')
    .trim();
}

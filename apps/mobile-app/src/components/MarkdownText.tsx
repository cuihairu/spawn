import * as Linking from 'expo-linking';
import { Pressable, StyleSheet, Text, View } from 'react-native';
import { colors } from '../constants/colors';
import { parseMarkdown, type Block, type InlineSegment } from '../lib/markdown';

// 依赖极简解析器（src/lib/markdown.ts）把 Markdown 子集渲染为 RN 原生
// Text/View——无 WebView、无 HTML 注入面。
const MarkdownText = ({ content }: { content: string }) => {
  const blocks = parseMarkdown(content);
  return (
    <View style={styles.container}>
      {blocks.map((block, index) => (
        <View key={index}>{renderBlock(block)}</View>
      ))}
    </View>
  );
};

function renderBlock(block: Block) {
  switch (block.type) {
    case 'heading':
      return (
        <Text style={block.level === 1 ? styles.h1 : block.level === 2 ? styles.h2 : styles.h3}>
          {renderInline(block.segments)}
        </Text>
      );
    case 'paragraph':
      return <Text style={styles.paragraph}>{renderInline(block.segments)}</Text>;
    case 'codeBlock':
      return (
        <View style={styles.codeBlock}>
          <Text style={styles.codeText}>{block.text}</Text>
        </View>
      );
    case 'list':
      return (
        <View style={styles.list}>
          {block.items.map((item, i) => (
            <Text key={i} style={styles.listItem}>
              {block.ordered ? `${i + 1}. ` : '•  '}
              {renderInline(item)}
            </Text>
          ))}
        </View>
      );
    case 'quote':
      return (
        <View style={styles.quote}>
          <Text style={styles.quoteText}>{renderInline(block.segments)}</Text>
        </View>
      );
    case 'hr':
      return <View style={styles.hr} />;
  }
}

function renderInline(segments: InlineSegment[]) {
  return segments.map((segment, index) => {
    switch (segment.type) {
      case 'bold':
        return <Text key={index} style={styles.bold}>{segment.text}</Text>;
      case 'italic':
        return <Text key={index} style={styles.italic}>{segment.text}</Text>;
      case 'code':
        return <Text key={index} style={styles.inlineCode}>{segment.text}</Text>;
      case 'link':
        return (
          <Pressable key={index} onPress={() => openLink(segment.href)}>
            <Text style={styles.link}>{segment.text || segment.href}</Text>
          </Pressable>
        );
      default:
        return <Text key={index}>{segment.text}</Text>;
    }
  });
}

// 仅放行 http(s) 外链与应用内路径（deep link），拒绝 javascript: 等危险 scheme。
function openLink(href: string) {
  if (/^https?:\/\//.test(href) || href.startsWith('/')) {
    void Linking.openURL(href).catch(() => {
      // 打不开的链接静默忽略
    });
  }
}

const styles = StyleSheet.create({
  container: {
    gap: 8,
  },
  h1: {
    color: colors.text,
    fontSize: 20,
    fontWeight: '700',
    marginTop: 4,
  },
  h2: {
    color: colors.text,
    fontSize: 18,
    fontWeight: '700',
    marginTop: 2,
  },
  h3: {
    color: colors.text,
    fontSize: 16,
    fontWeight: '600',
  },
  paragraph: {
    color: colors.text,
    fontSize: 15,
    lineHeight: 24,
  },
  codeBlock: {
    backgroundColor: colors.card,
    borderRadius: 8,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
    padding: 10,
  },
  codeText: {
    color: colors.text,
    fontFamily: 'monospace',
    fontSize: 13,
    lineHeight: 20,
  },
  list: {
    gap: 4,
  },
  listItem: {
    color: colors.text,
    fontSize: 15,
    lineHeight: 24,
  },
  quote: {
    borderLeftWidth: 3,
    borderLeftColor: colors.primary,
    paddingLeft: 10,
  },
  quoteText: {
    color: colors.textMuted,
    fontSize: 15,
    lineHeight: 24,
  },
  hr: {
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: colors.border,
    marginVertical: 4,
  },
  bold: {
    fontWeight: '700',
  },
  italic: {
    fontStyle: 'italic',
  },
  inlineCode: {
    fontFamily: 'monospace',
    fontSize: 13,
    color: colors.primary,
    backgroundColor: colors.card,
  },
  link: {
    color: colors.primary,
    textDecorationLine: 'underline',
  },
});

export default MarkdownText;

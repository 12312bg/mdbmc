<script setup lang="ts">
import { computed } from 'vue'
import MarkdownIt from 'markdown-it'
import { api } from '@/api'

// Renders release notes. Raw HTML is disabled, so the notes cannot inject
// markup; links open in the system browser instead of navigating the
// program's window away.
const props = defineProps<{ source: string }>()

const md = new MarkdownIt({ html: false, linkify: true, breaks: true })
const html = computed(() => md.render(props.source || ''))

function onClick(e: MouseEvent) {
  const a = (e.target as HTMLElement).closest('a')
  if (!a) return
  e.preventDefault()
  const href = a.getAttribute('href') ?? ''
  if (/^https?:\/\//i.test(href)) api.openExternal(href)
}
</script>

<template>
  <div class="md" @click="onClick" v-html="html" />
</template>

<style scoped>
.md {
  font-size: 13.5px;
  line-height: 1.7;
  color: var(--text-2);
  word-break: break-word;
}
.md :deep(> :first-child) {
  margin-top: 0;
}
.md :deep(> :last-child) {
  margin-bottom: 0;
}
.md :deep(h1),
.md :deep(h2),
.md :deep(h3),
.md :deep(h4) {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 18px 0 8px;
  color: var(--text-1);
  font-size: 14px;
  font-weight: 600;
  line-height: 20px;
}
.md :deep(h1)::before,
.md :deep(h2)::before {
  content: '';
  width: 3px;
  height: 14px;
  border-radius: 2px;
  background: var(--accent);
}
.md :deep(h3),
.md :deep(h4) {
  font-size: 13.5px;
}
.md :deep(p) {
  margin: 6px 0;
}
.md :deep(ul),
.md :deep(ol) {
  margin: 6px 0;
  padding-left: 20px;
}
.md :deep(li) {
  margin: 3px 0;
}
.md :deep(li::marker) {
  color: var(--text-3);
}
.md :deep(strong) {
  color: var(--text-1);
  font-weight: 600;
}
.md :deep(a) {
  color: var(--accent);
  text-decoration: none;
}
.md :deep(a:hover) {
  text-decoration: underline;
}
.md :deep(code) {
  font-family: 'Cascadia Mono', Consolas, 'SFMono-Regular', Menlo, monospace;
  font-size: 12px;
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--surface-2);
  border: 1px solid var(--border);
}
.md :deep(pre) {
  margin: 8px 0;
  padding: 10px 12px;
  border-radius: 6px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  overflow-x: auto;
}
.md :deep(pre code) {
  padding: 0;
  border: 0;
  background: none;
}
.md :deep(blockquote) {
  margin: 8px 0;
  padding: 2px 12px;
  border-left: 3px solid var(--border);
  color: var(--text-3);
}
.md :deep(hr) {
  border: 0;
  border-top: 1px solid var(--border);
  margin: 14px 0;
}
.md :deep(table) {
  border-collapse: collapse;
  margin: 8px 0;
  font-size: 12.5px;
}
.md :deep(th),
.md :deep(td) {
  border: 1px solid var(--border);
  padding: 4px 10px;
}
</style>

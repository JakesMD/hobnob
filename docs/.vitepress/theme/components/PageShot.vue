<script setup lang="ts">
// hobnob's browser page held on one prompt, for the landing page. Picking an
// answer puts the same prompt back, so it stays a picture of that moment.
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { PAGE } from './page'

const props = defineProps<{ task: string; info: string; varName: string; question: string; options: string[] }>()

const frame = ref<HTMLIFrameElement>()
let id = 0

function send() {
  const snapshot = {
    screen: 'prompt',
    task: { name: props.task, info: props.info },
    rail: { answers: [], setup: [] },
    prompt: { id: ++id, kind: 'menu', varName: props.varName, info: props.question, options: props.options, multi: false },
  }
  frame.value?.contentWindow?.postMessage({ hobnobSnapshot: JSON.stringify(snapshot) }, '*')
}

function onMessage(e: MessageEvent) {
  if (frame.value && e.source === frame.value.contentWindow && e.data) send()
}

onMounted(() => window.addEventListener('message', onMessage))
onBeforeUnmount(() => window.removeEventListener('message', onMessage))
</script>

<template>
  <div class="shot">
    <div class="bar"><span /><span /><span /><p>localhost · hobnob --browser</p></div>
    <iframe ref="frame" :srcdoc="PAGE" title="hobnob's browser page, asking which disk to flash" tabindex="-1" />
  </div>
</template>

<style scoped>
.shot {
  display: flex;
  flex-direction: column;
  height: 500px;
  overflow: hidden;
  border: 1px solid var(--vp-c-divider);
  border-radius: 12px;
  background: #1a2029;
}
.bar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--vp-c-divider);
  background: #202733;
}
.bar span {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: #3a4453;
}
.bar p {
  margin: 0 0 0 10px;
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  line-height: 1;
  color: var(--vp-c-text-3);
}
iframe {
  flex: 1;
  width: 100%;
  border: none;
  color-scheme: dark;
}
</style>

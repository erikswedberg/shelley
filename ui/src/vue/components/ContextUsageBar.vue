<!-- Vue port of the ContextUsageBar inner component from ChatInterface.tsx,
     now backed by PrimeVue Popover (outside-click dismissal and positioning
     come for free — the manual document click listener and fixed-position
     math are gone). Preserves the context-usage-bar / chat-context-popup /
     chat-distill-* class contract. Auto-opens once per browser on the
     long-conversation threshold. -->
<template>
  <div ref="barRef">
    <Popover
      ref="popoverRef"
      :pt="{
        root: { class: 'chat-context-popup' },
        content: { class: 'chat-context-popup-content' },
      }"
    >
      <div v-if="modelName" class="chat-popup-model-name">{{ modelName }}</div>
      {{ formatTokens(contextWindowSize) }} / {{ formatTokens(maxContextTokens) }} ({{
        percentage.toFixed(1)
      }}%) tokens used
      <TokenCostGraph
        v-if="tokenGraphEnabled"
        :entries="usageEntries || []"
        :conversation-id="conversationId"
      />
      <div v-if="showLongConversationWarning" class="chat-popup-warning">
        This conversation is getting long.
        <br />
        For best results, start a new conversation.
      </div>
      <div
        v-if="conversationId && (onDistillNewGeneration || onStartNewGeneration)"
        class="chat-distill-container"
      >
        <button
          v-if="onDistillNewGeneration"
          :disabled="distilling"
          class="chat-distill-button chat-distill-generation-button"
          @click="handleDistillNewGeneration"
        >
          {{ distilling ? "Compacting..." : "Compact Conversation" }}
        </button>
        <button
          v-if="onStartNewGeneration"
          :disabled="distilling"
          class="chat-distill-button chat-distill-generation-button"
          @click="handleStartNewGeneration"
        >
          Start New Generation
        </button>
      </div>
    </Popover>
    <div class="context-usage-bar-container">
      <span
        v-if="showLongConversationWarning"
        class="context-warning-icon"
        title="This conversation is getting long. For best results, start a new conversation."
      >
        ⚠️
      </span>
      <div
        class="context-usage-bar"
        :title="`Context: ${formatTokens(contextWindowSize)} / ${formatTokens(maxContextTokens)} tokens (${percentage.toFixed(1)}%)`"
        @click="popoverRef?.toggle($event)"
      >
        <div
          class="context-usage-fill"
          :style="{ width: clampedPercentage + '%', backgroundColor: barColor }"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import Popover from "primevue/popover";
import type { UsageEntry } from "../../utils/tokenCostGraph";
import { useFeatureFlag } from "../composables/featureFlags";
import TokenCostGraph from "./TokenCostGraph.vue";

const props = defineProps<{
  contextWindowSize: number;
  maxContextTokens: number;
  conversationId?: string | null;
  modelName?: string;
  usageEntries?: UsageEntry[];
  onDistillNewGeneration?: () => Promise<void> | void;
  onStartNewGeneration?: () => Promise<void> | void;
  agentWorking?: boolean;
}>();

const tokenGraphEnabled = useFeatureFlag("token-cost-graph");

const distilling = ref(false);
const barRef = ref<HTMLDivElement | null>(null);
const popoverRef = ref<InstanceType<typeof Popover> | null>(null);
let hasAutoOpened = false;

const percentage = computed(() =>
  props.maxContextTokens > 0 ? (props.contextWindowSize / props.maxContextTokens) * 100 : 0,
);
const clampedPercentage = computed(() => Math.min(percentage.value, 100));
const showLongConversationWarning = computed(
  () => props.maxContextTokens > 0 && props.contextWindowSize >= props.maxContextTokens * 0.5,
);

const barColor = computed(() => {
  if (percentage.value >= 90) return "var(--error-text)";
  if (percentage.value >= 70) return "var(--warning-text, #f59e0b)";
  return "var(--blue-text)";
});

function formatTokens(tokens: number): string {
  if (tokens >= 1000000) return `${(tokens / 1000000).toFixed(1)}M`;
  if (tokens >= 1000) return `${(tokens / 1000).toFixed(0)}k`;
  return tokens.toString();
}

// Auto-open popup once per browser at the long-conversation threshold.
// Programmatic open: PrimeVue's show() anchors to event.currentTarget, so pass
// the usage bar element explicitly as the target.
watch(
  [showLongConversationWarning, () => props.agentWorking, () => props.conversationId],
  () => {
    const isMobile = window.innerWidth <= 768;
    if (
      showLongConversationWarning.value &&
      !props.agentWorking &&
      !isMobile &&
      props.conversationId &&
      !hasAutoOpened &&
      localStorage.getItem("shelley_long_convo_popup_shown") !== "1"
    ) {
      hasAutoOpened = true;
      // Wait a tick: with { immediate: true } this can fire before mount,
      // when barRef/popoverRef are still null. Only burn the once-per-browser
      // localStorage flag if the popup actually opens.
      void nextTick(() => {
        const anchor = barRef.value?.querySelector<HTMLElement>(".context-usage-bar");
        if (!anchor || !popoverRef.value) return;
        localStorage.setItem("shelley_long_convo_popup_shown", "1");
        popoverRef.value.show(new Event("click"), anchor);
      });
    }
  },
  { immediate: true },
);

async function handleDistillNewGeneration() {
  if (distilling.value || !props.onDistillNewGeneration) return;
  distilling.value = true;
  try {
    await props.onDistillNewGeneration();
    popoverRef.value?.hide();
  } finally {
    distilling.value = false;
  }
}

async function handleStartNewGeneration() {
  if (distilling.value || !props.onStartNewGeneration) return;
  distilling.value = true;
  try {
    await props.onStartNewGeneration();
    popoverRef.value?.hide();
  } finally {
    distilling.value = false;
  }
}
</script>

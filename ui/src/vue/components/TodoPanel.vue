<template>
  <div v-if="visible" class="todo-panel" data-testid="todo-panel">
    <div class="todo-panel-header">
      <div class="todo-panel-header-left">
        <svg
          class="todo-panel-icon"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          width="16"
          height="16"
        >
          <path d="M9 11l3 3L22 4" />
          <path d="M21 12v7a2 2 0 01-2 2H5a2 2 0 01-2-2V5a2 2 0 012-2h11" />
        </svg>
        <span>Working...</span>
        <span class="todo-panel-count">{{ completedCount }}/{{ totalCount }}</span>
      </div>
      <div class="todo-panel-header-right">
        <button
          class="todo-panel-minimize"
          :title="minimized ? 'Expand' : 'Minimize'"
          @click="$emit('toggleMinimize')"
        >
          {{ minimized ? "+" : "-" }}
        </button>
        <button
          v-if="allCompleted"
          class="todo-panel-dismiss"
          title="Dismiss"
          @click="$emit('dismiss')"
        >
          ×
        </button>
      </div>
    </div>
    <div v-if="!minimized" class="todo-panel-items">
      <div v-for="item in items" :key="item.id" :class="`todo-item todo-item-${item.status}`">
        <span class="todo-item-icon">
          <svg
            v-if="item.status === 'completed'"
            width="14"
            height="14"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M5 13l4 4L19 7" />
          </svg>
          <svg
            v-else-if="item.status === 'in-progress'"
            width="14"
            height="14"
            viewBox="0 0 24 24"
          >
            <circle cx="12" cy="12" r="5" fill="currentColor" />
          </svg>
          <svg
            v-else
            width="14"
            height="14"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
          >
            <circle cx="12" cy="12" r="5" />
          </svg>
        </span>
        <span class="todo-item-text">{{ item.task }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";

export interface TodoItem {
  id: string;
  task: string;
  status: string;
}

const props = defineProps<{
  todoContent: string;
  dismissed: boolean;
  minimized: boolean;
}>();

defineEmits<{
  toggleMinimize: [];
  dismiss: [];
}>();

const items = computed<TodoItem[]>(() => {
  if (!props.todoContent.trim()) return [];
  try {
    const parsed = JSON.parse(props.todoContent) as { items?: TodoItem[] };
    return parsed.items ?? [];
  } catch {
    return [];
  }
});

const totalCount = computed(() => items.value.length);
const completedCount = computed(() => items.value.filter((i) => i.status === "completed").length);
const allCompleted = computed(() => completedCount.value === totalCount.value);
const visible = computed(() => !props.dismissed && items.value.length > 0);
</script>

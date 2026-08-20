<!--
Copyright (c) 2025 The Linux Foundation and each contributor.
SPDX-License-Identifier: MIT
-->
<template>
  <div
    :id="props.id"
    ref="scrollViewRef"
    class="scroll-view"
  >
    <slot />
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue';

const scrollViewRef = ref<HTMLElement | null>(null);
const props = defineProps<{
  id: string;
  observer: IntersectionObserver | null;
}>();

const emit = defineEmits<{ (e: 'scrolledToView', id: string): void }>();

const onScrolledToView = () => {
  emit('scrolledToView', props.id);
};

function observe(observer: IntersectionObserver | null) {
  if (observer && scrollViewRef.value) {
    observer.observe(scrollViewRef.value);
  }
}

function unobserve(observer: IntersectionObserver | null) {
  if (observer && scrollViewRef.value) {
    observer.unobserve(scrollViewRef.value);
  }
}

onMounted(() => {
  if (scrollViewRef.value) {
    scrollViewRef.value.addEventListener('scrolledToView', onScrolledToView);
  }
  observe(props.observer);
});

onUnmounted(() => {
  if (scrollViewRef.value) {
    scrollViewRef.value.removeEventListener('scrolledToView', onScrolledToView);
  }
  unobserve(props.observer);
});

watch(
  () => props.observer,
  (observer, previous) => {
    unobserve(previous ?? null);
    observe(observer);
  },
);
</script>
<script lang="ts">
export default {
  name: 'LfxScrollView',
};
</script>

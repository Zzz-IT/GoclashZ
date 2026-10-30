<template>
  <div class="toast-stack" aria-live="polite">
    <TransitionGroup name="toast">
      <div
        v-for="item in globalState.toasts"
        :key="item.id"
        class="toast"
        :class="'toast--' + item.kind"
      >
        <span class="toast__bar"></span>
        <div class="toast__body">
          <div v-if="item.title" class="toast__title">{{ item.title }}</div>
          <div class="toast__text">{{ item.message }}</div>
        </div>
        <button class="toast__close" @click="dismissToast(item.id)">×</button>
      </div>
    </TransitionGroup>
  </div>
</template>

<script setup lang="ts">
import { globalState, dismissToast } from '../store';
</script>

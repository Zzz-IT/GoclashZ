<template>
  <div class="modern-card">
    <div class="card-content">
      <slot></slot>
    </div>
  </div>
</template>

<style scoped>
.modern-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-xl);
  padding: 20px;
  box-shadow: var(--shadow-main);
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}
.modern-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 10px 15px -3px rgb(0 0 0 / 0.1);
}
</style>
<script setup lang="ts">
const states = [
  ['ready', 'Ready in repo', 'Implemented and validated locally'],
  ['active', 'In progress', 'Physical work is underway'],
  ['planned', 'Not deployed', 'Designed but not verified on Titan'],
  ['blocked', 'Blocked', 'Waiting for a prerequisite or decision'],
  ['legacy', 'Legacy review', 'Inherited and unsafe to apply unchanged'],
]
</script>

<template>
  <div class="status-legend" aria-label="Project status legend">
    <div v-for="state in states" :key="state[0]" class="status-item">
      <span :class="['status-dot', state[0]]" />
      <span><strong>{{ state[1] }}</strong><small>{{ state[2] }}</small></span>
    </div>
  </div>
</template>

<style scoped>
.status-legend {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin: 16px 0 28px;
}

.status-item {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 11px 13px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 11px;
  background: var(--vp-c-bg-soft);
}

.status-item > span:last-child { display: grid; gap: 1px; }
.status-item strong { color: var(--vp-c-text-1); font-size: 12px; }
.status-item small { color: var(--vp-c-text-3); font-size: 10px; }
.status-dot { width: 9px; height: 9px; flex: 0 0 auto; border-radius: 50%; background: var(--vp-c-text-3); box-shadow: 0 0 0 4px color-mix(in srgb, currentColor 10%, transparent); }
.status-dot.ready { color: var(--homelab-success); background: var(--homelab-success); }
.status-dot.active { color: var(--homelab-warning); background: var(--homelab-warning); }
.status-dot.planned { color: var(--homelab-info); background: var(--homelab-info); }
.status-dot.blocked { color: var(--homelab-danger); background: var(--homelab-danger); }
.status-dot.legacy { color: var(--vp-c-text-3); background: var(--vp-c-text-3); }

@media (max-width: 640px) {
  .status-legend { grid-template-columns: 1fr; }
}
</style>

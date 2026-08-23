<script setup lang="ts">
defineProps<{
  checkpoints: number
  description: string
  label: string
}>()
</script>

<template>
  <section class="runbook-hero" aria-label="Runbook summary">
    <div class="runbook-identity" aria-hidden="true">TI</div>
    <div class="runbook-copy">
      <div class="runbook-meta">
        <span class="runbook-label">{{ label }}</span>
        <span class="runbook-state"><i /> Ready in repo</span>
      </div>
      <p>{{ description }}</p>
      <div class="runbook-facts">
        <span><strong>{{ checkpoints }}</strong> gated checkpoints</span>
        <span><strong>1</strong> K3s server</span>
        <span><strong>0</strong> public management ports</span>
      </div>
    </div>
  </section>
</template>

<style scoped>
.runbook-hero {
  position: relative;
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 22px;
  margin: 22px 0 34px;
  padding: 24px;
  overflow: hidden;
  border: 1px solid var(--vp-c-divider);
  border-radius: 18px;
  background:
    radial-gradient(circle at 100% 0, color-mix(in srgb, var(--vp-c-brand-1) 20%, transparent), transparent 42%),
    var(--vp-c-bg-soft);
  box-shadow: var(--homelab-shadow);
}

.runbook-hero::after {
  position: absolute;
  right: -22px;
  bottom: -38px;
  width: 150px;
  height: 150px;
  border: 1px solid color-mix(in srgb, var(--vp-c-brand-1) 25%, transparent);
  border-radius: 50%;
  content: '';
}

.runbook-identity {
  display: grid;
  place-items: center;
  width: 56px;
  height: 56px;
  border-radius: 15px;
  color: #fff;
  background: linear-gradient(145deg, var(--vp-c-brand-1), var(--homelab-navy));
  box-shadow: 0 10px 24px color-mix(in srgb, var(--vp-c-brand-1) 24%, transparent);
  font-size: 15px;
  font-weight: 800;
  letter-spacing: .14em;
}

.runbook-copy { min-width: 0; }

.runbook-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.runbook-label {
  color: var(--vp-c-brand-1);
  font-size: 12px;
  font-weight: 750;
  letter-spacing: .12em;
  text-transform: uppercase;
}

.runbook-state {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  color: var(--homelab-success);
  font-size: 12px;
  font-weight: 700;
}

.runbook-state i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: currentColor;
  box-shadow: 0 0 0 4px color-mix(in srgb, currentColor 13%, transparent);
}

.runbook-copy p {
  max-width: 680px;
  margin: 9px 0 18px;
  color: var(--vp-c-text-1);
  font-size: 16px;
  line-height: 1.65;
}

.runbook-facts {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.runbook-facts span {
  padding: 6px 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 999px;
  color: var(--vp-c-text-2);
  background: color-mix(in srgb, var(--vp-c-bg) 75%, transparent);
  font-size: 12px;
}

.runbook-facts strong { color: var(--vp-c-text-1); }

@media (max-width: 640px) {
  .runbook-hero { grid-template-columns: 1fr; padding: 20px; }
  .runbook-meta { align-items: flex-start; flex-direction: column; }
}
</style>

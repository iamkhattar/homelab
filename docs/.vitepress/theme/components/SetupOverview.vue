<script setup lang="ts">
type Phase = {
  eyebrow: string
  title: string
  detail: string
  href: string
}

defineProps<{ phases: Phase[] }>()
</script>

<template>
  <nav class="setup-overview" aria-label="Titan setup phases">
    <a v-for="(phase, index) in phases" :key="phase.href" :href="phase.href" class="setup-phase">
      <span class="phase-number">{{ String(index + 1).padStart(2, '0') }}</span>
      <span class="phase-copy">
        <small>{{ phase.eyebrow }}</small>
        <strong>{{ phase.title }}</strong>
        <span>{{ phase.detail }}</span>
      </span>
      <span class="phase-arrow" aria-hidden="true">→</span>
    </a>
  </nav>
</template>

<style scoped>
.setup-overview {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin: 18px 0 38px;
}

.setup-phase {
  display: grid;
  grid-template-columns: auto 1fr auto;
  gap: 13px;
  align-items: center;
  min-height: 112px;
  padding: 17px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 14px;
  color: inherit;
  background: var(--vp-c-bg-soft);
  text-decoration: none;
  transition: border-color .2s ease, box-shadow .2s ease, transform .2s ease;
}

.setup-phase:hover {
  border-color: var(--vp-c-brand-1);
  box-shadow: var(--homelab-shadow);
  transform: translateY(-2px);
}

.phase-number {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  border-radius: 10px;
  color: var(--vp-c-brand-1);
  background: color-mix(in srgb, var(--vp-c-brand-1) 11%, transparent);
  font: 700 12px/1 var(--vp-font-family-mono);
}

.phase-copy { display: grid; gap: 3px; min-width: 0; }
.phase-copy small { color: var(--vp-c-text-3); font-size: 10px; font-weight: 700; letter-spacing: .1em; text-transform: uppercase; }
.phase-copy strong { color: var(--vp-c-text-1); font-size: 14px; }
.phase-copy span { color: var(--vp-c-text-2); font-size: 12px; line-height: 1.45; }
.phase-arrow { color: var(--vp-c-brand-1); font-size: 18px; transition: transform .2s ease; }
.setup-phase:hover .phase-arrow { transform: translateX(3px); }

@media (max-width: 700px) {
  .setup-overview { grid-template-columns: 1fr; }
}
</style>

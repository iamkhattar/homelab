<template>
  <div class="foundation-map" role="img" aria-label="Operator workstation connects privately to Titan, which exports recovery data to encrypted off-node storage">
    <div class="map-node">
      <span class="map-icon">⌘</span>
      <small>Operator</small>
      <strong>Workstation</strong>
      <em>homelabctl + SSH</em>
    </div>
    <div class="map-link">
      <span>private LAN</span>
      <i>→</i>
    </div>
    <div class="map-node titan">
      <span class="map-icon">TI</span>
      <small>Single node</small>
      <strong>Titan</strong>
      <em>Debian · K3s · etcd</em>
    </div>
    <div class="map-link">
      <span>encrypted export</span>
      <i>→</i>
    </div>
    <div class="map-node">
      <span class="map-icon">↗</span>
      <small>Recovery</small>
      <strong>Off-node copy</strong>
      <em>snapshot + token</em>
    </div>
  </div>
</template>

<style scoped>
.foundation-map {
  display: grid;
  grid-template-columns: 1fr auto 1fr auto 1fr;
  align-items: center;
  gap: 12px;
  margin: 20px 0 32px;
  padding: 20px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 18px;
  background:
    linear-gradient(135deg, color-mix(in srgb, var(--vp-c-brand-1) 7%, transparent), transparent 38%),
    var(--vp-c-bg-soft);
}

.map-node {
  display: grid;
  justify-items: center;
  min-width: 0;
  padding: 17px 10px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 14px;
  background: var(--vp-c-bg);
  text-align: center;
}

.map-node.titan {
  border-color: color-mix(in srgb, var(--vp-c-brand-1) 55%, var(--vp-c-divider));
  box-shadow: 0 10px 26px color-mix(in srgb, var(--vp-c-brand-1) 10%, transparent);
}

.map-icon {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  margin-bottom: 10px;
  border-radius: 11px;
  color: var(--vp-c-brand-1);
  background: color-mix(in srgb, var(--vp-c-brand-1) 11%, transparent);
  font: 750 11px/1 var(--vp-font-family-mono);
}

.map-node small { color: var(--vp-c-text-3); font-size: 10px; letter-spacing: .08em; text-transform: uppercase; }
.map-node strong { margin-top: 3px; color: var(--vp-c-text-1); font-size: 14px; }
.map-node em { margin-top: 4px; color: var(--vp-c-text-2); font-size: 11px; font-style: normal; }
.map-link { display: grid; justify-items: center; color: var(--vp-c-text-3); }
.map-link span { font-size: 9px; letter-spacing: .06em; text-transform: uppercase; white-space: nowrap; }
.map-link i { color: var(--vp-c-brand-1); font-size: 20px; font-style: normal; }

@media (max-width: 760px) {
  .foundation-map { grid-template-columns: 1fr; }
  .map-link i { transform: rotate(90deg); }
}
</style>

import DefaultTheme from 'vitepress/theme'
import type { Theme } from 'vitepress'
import FoundationMap from './components/FoundationMap.vue'
import RunbookHero from './components/RunbookHero.vue'
import SetupOverview from './components/SetupOverview.vue'
import StatusLegend from './components/StatusLegend.vue'
import './styles.css'

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component('FoundationMap', FoundationMap)
    app.component('RunbookHero', RunbookHero)
    app.component('SetupOverview', SetupOverview)
    app.component('StatusLegend', StatusLegend)
  },
} satisfies Theme

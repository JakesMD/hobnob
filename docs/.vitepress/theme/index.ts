import DefaultTheme from 'vitepress/theme'
import type { Theme } from 'vitepress'
import TryDemo from './components/TryDemo.vue'
import './custom.css'

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component('TryDemo', TryDemo)
  },
} satisfies Theme

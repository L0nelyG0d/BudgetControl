import '@testing-library/jest-dom/vitest'
import { beforeEach } from 'vitest'
import { resetConfigForTests } from '@/lib/config'

// The amount limit is cached per page load; every test starts a fresh page.
beforeEach(resetConfigForTests)

// jsdom has no matchMedia; sonner's Toaster and the theme toggle read it.
if (typeof window.matchMedia !== 'function') {
  window.matchMedia = (query: string) =>
    ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    }) as MediaQueryList
}

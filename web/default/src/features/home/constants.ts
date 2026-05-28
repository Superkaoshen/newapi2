/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
/**
 * Home page constants for Tuyao API landing page
 */
import { type TFunction } from 'i18next'

export const MAIN_BASE_CLASSES = 'bg-background text-foreground w-full'

export const HERO_STAT_ITEMS = [
  {
    value: 99.9,
    suffix: '%',
    label: 'Online processing availability',
    decimals: 1,
  },
  { value: 3, suffix: 's', label: 'Average print extraction preview' },
  { value: '24/7', label: 'API access for production workflows' },
] as const

export const FEATURE_CARDS = [
  {
    title: 'AI print extraction',
    description:
      'Upload design references and extract repeatable print elements from source images.',
    iconName: 'ScanSearch',
  },
  {
    title: 'Template matching',
    description:
      'Detect pattern structure, borders, and layout details for accurate print replication.',
    iconName: 'Layers3',
  },
  {
    title: 'Batch processing',
    description:
      'Handle multiple assets in a single flow for studios, factories, and e-commerce teams.',
    iconName: 'Files',
  },
  {
    title: 'API integration',
    description:
      'Connect extraction results to your product pipeline with a clean API-first workflow.',
    iconName: 'Code2',
  },
] as const

export const WORKFLOW_STEPS = [
  {
    title: 'Upload source artwork',
    description:
      'Submit print references, fabric photos, sketches, or sample images.',
  },
  {
    title: 'AI extracts print layers',
    description:
      'Separate pattern, contour, color blocks, and repeatable design elements.',
  },
  {
    title: 'Export and integrate',
    description:
      'Receive structured output for review, production, or downstream API use.',
  },
] as const

export const USE_CASES = [
  'Fashion print production',
  'Custom merchandise design',
  'Pattern library management',
  'Print asset automation',
] as const

export const BENEFIT_ITEMS = [
  'Fast preview for print extraction',
  'Consistent layout recognition',
  'Built for production workflows',
  'Easy API integration for teams',
] as const

export const DEFAULT_STATS = [
  {
    value: '3',
    suffix: '+',
    description: 'print extraction input types',
  },
  {
    value: '4',
    suffix: '+',
    description: 'production output stages',
  },
  {
    value: '10',
    suffix: '+',
    description: 'compatible workflow routes',
  },
  {
    value: '24',
    suffix: '/7',
    description: 'async API availability',
  },
] as const

export const DEFAULT_FEATURES = [
  {
    title: 'Print Extraction First',
    description:
      'Built around AI extraction of repeatable print elements from source imagery',
    iconName: 'ScanSearch',
  },
  {
    title: 'Production Workflow',
    description:
      'Turn references into clean assets for design review and downstream production',
    iconName: 'Workflow',
  },
  {
    title: 'Batch Asset Handling',
    description: 'Process multiple print references through a consistent API workflow',
    iconName: 'Files',
  },
  {
    title: 'API Native',
    description:
      'Integrate extraction jobs, status polling, and result delivery into existing tools',
    iconName: 'Code',
  },
  {
    title: 'Structured Results',
    description:
      'Return preview and output metadata that are easier for teams to consume',
    iconName: 'Layers3',
  },
  {
    title: 'Usage Visibility',
    description:
      'Track asynchronous extraction jobs, progress, and generated results in logs',
    iconName: 'Gauge',
  },
  {
    title: 'Team Operations',
    description:
      'Support studio, factory, and e-commerce teams with shared API access',
    iconName: 'Users',
  },
  {
    title: 'Stable Access',
    description:
      'Keep print extraction available for production-oriented workflows',
    iconName: 'Shield',
  },
] as const

export function getDefaultStats(t: TFunction) {
  return DEFAULT_STATS.map((stat) => ({
    ...stat,
    description: stat.description ? t(stat.description) : undefined,
  }))
}

export function getDefaultFeatures(t: TFunction) {
  return DEFAULT_FEATURES.map((feature) => ({
    ...feature,
    title: t(feature.title),
    description: t(feature.description),
  }))
}

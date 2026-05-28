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
import { Code2, Files, Layers3, ScanSearch } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { AnimateInView } from '@/components/animate-in-view'
import { FEATURE_CARDS } from '../../constants'

const ICONS = {
  ScanSearch: <ScanSearch className='size-4 text-orange-500' />,
  Layers3: <Layers3 className='size-4 text-pink-500' />,
  Files: <Files className='size-4 text-violet-500' />,
  Code2: <Code2 className='size-4 text-sky-500' />,
} as const

export function Features() {
  const { t } = useTranslation()

  return (
    <section className='relative z-10 px-6 py-24 md:py-32'>
      <div className='mx-auto max-w-6xl'>
        <AnimateInView className='mb-16 max-w-lg'>
          <p className='text-muted-foreground mb-3 text-xs font-medium tracking-widest uppercase'>
            {t('Core capabilities')}
          </p>
          <h2 className='text-2xl leading-tight font-bold tracking-tight md:text-3xl'>
            {t('Turn source images into production-ready print assets')}
          </h2>
        </AnimateInView>

        <div className='border-border/40 bg-border/40 grid gap-px overflow-hidden rounded-xl border md:grid-cols-2'>
          {FEATURE_CARDS.map((feature, index) => (
            <FeatureCard
              key={feature.title}
              feature={feature}
              index={index}
            />
          ))}
        </div>
      </div>
    </section>
  )
}

function FeatureCard(props: {
  feature: (typeof FEATURE_CARDS)[number]
  index: number
}) {
  const { t } = useTranslation()
  const icon = ICONS[props.feature.iconName as keyof typeof ICONS]
  const tags = getTags(props.index)

  return (
    <AnimateInView
      delay={props.index * 100}
      animation='scale-in'
      className='bg-background group hover:bg-muted/20 p-7 transition-colors duration-300 md:p-8'
    >
      <div className='mb-3 flex items-center gap-3'>
        <span className='border-border/40 bg-muted text-muted-foreground flex size-7 items-center justify-center rounded-md border text-[10px] font-semibold tabular-nums'>
          0{props.index + 1}
        </span>
        {icon}
        <h3 className='text-sm font-semibold'>{t(props.feature.title)}</h3>
      </div>
      <p className='text-muted-foreground text-sm leading-relaxed'>
        {t(props.feature.description)}
      </p>
      <div className='mt-4 grid grid-cols-2 gap-2 text-xs'>
        {tags.map((tag) => (
          <div
            key={tag}
            className='border-border/30 bg-muted/20 rounded-lg border px-3 py-2'
          >
            {t(tag)}
          </div>
        ))}
      </div>
    </AnimateInView>
  )
}

function getTags(index: number) {
  const tagSets = [
    ['Photo input', 'Sketch input', 'Reference photo', 'Repeatable output'],
    ['Borders', 'Contours', 'Color blocks', 'Layout map'],
    ['Bulk jobs', 'Queue control', 'Studio team', 'Factory workflow'],
    ['REST API', 'JSON output', 'Webhook delivery', 'Team integration'],
  ] as const

  return tagSets[index] ?? tagSets[0]
}

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
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'

type AccentTone = 'orange' | 'pink' | 'violet'

interface DemoConfig {
  id: string
  label: string
  badge: string
  title: string
  request: string[]
  response: string[]
  accent: AccentTone
  latency: number
  confidence: number
}

const ACCENT_CLASSES: Record<
  AccentTone,
  {
    activeText: string
    activeBorder: string
    badge: string
  }
> = {
  orange: {
    activeText: 'text-orange-600 dark:text-orange-400',
    activeBorder: 'border-orange-500 dark:border-orange-400',
    badge:
      'bg-orange-500/10 text-orange-600 dark:bg-orange-400/10 dark:text-orange-400',
  },
  pink: {
    activeText: 'text-pink-600 dark:text-pink-400',
    activeBorder: 'border-pink-500 dark:border-pink-400',
    badge:
      'bg-pink-500/10 text-pink-600 dark:bg-pink-400/10 dark:text-pink-400',
  },
  violet: {
    activeText: 'text-violet-600 dark:text-violet-400',
    activeBorder: 'border-violet-500 dark:border-violet-400',
    badge:
      'bg-violet-500/10 text-violet-600 dark:bg-violet-400/10 dark:text-violet-400',
  },
}

const DEMOS: DemoConfig[] = [
  {
    id: 'upload',
    label: 'Upload',
    badge: 'source image',
    title: 'Collect print references from photos or sketches',
    request: [
      '{',
      '  "asset": "fabric-photo.jpg",',
      '  "mode": "print_extraction",',
      '  "target": "repeatable pattern"',
      '}',
    ],
    response: [
      '{',
      '  "status": "queued",',
      '  "preview": "generated",',
      '  "layers": 3',
      '}',
    ],
    accent: 'orange',
    latency: 86,
    confidence: 92,
  },
  {
    id: 'extract',
    label: 'Extract',
    badge: 'ai analysis',
    title: 'Detect layout, border, and repeatable print layers',
    request: [
      '{',
      '  "detect": ["pattern", "outline", "color-block"],',
      '  "accuracy": "high",',
      '  "return": "structured print map"',
      '}',
    ],
    response: [
      '{',
      '  "pattern": "separated",',
      '  "outline": "normalized",',
      '  "confidence": 0.94',
      '}',
    ],
    accent: 'pink',
    latency: 112,
    confidence: 94,
  },
  {
    id: 'export',
    label: 'Export',
    badge: 'api ready',
    title: 'Return clean output for production workflows',
    request: [
      '{',
      '  "format": "json",',
      '  "destination": "design-pipeline",',
      '  "delivery": "api"',
      '}',
    ],
    response: [
      '{',
      '  "export": "completed",',
      '  "assets": 5,',
      '  "ready": true',
      '}',
    ],
    accent: 'violet',
    latency: 64,
    confidence: 97,
  },
]

const CYCLE_INTERVAL = 4200
const TRANSITION_MS = 220

interface HeroTerminalDemoProps {
  className?: string
}

export function HeroTerminalDemo(props: HeroTerminalDemoProps) {
  const { t } = useTranslation()
  const [activeIndex, setActiveIndex] = useState(0)
  const [transitioning, setTransitioning] = useState(false)
  const intervalRef = useRef<ReturnType<typeof setInterval>>(undefined)
  const timeoutRef = useRef<ReturnType<typeof setTimeout>>(undefined)

  useEffect(() => {
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    if (mq.matches) return

    intervalRef.current = setInterval(() => {
      setTransitioning(true)
      timeoutRef.current = setTimeout(() => {
        setActiveIndex((prev) => (prev + 1) % DEMOS.length)
        setTransitioning(false)
      }, TRANSITION_MS)
    }, CYCLE_INTERVAL)

    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current)
      if (timeoutRef.current) clearTimeout(timeoutRef.current)
    }
  }, [])

  const handleSelect = (index: number) => {
    if (index === activeIndex) return
    if (intervalRef.current) clearInterval(intervalRef.current)
    if (timeoutRef.current) clearTimeout(timeoutRef.current)
    setTransitioning(true)
    timeoutRef.current = setTimeout(() => {
      setActiveIndex(index)
      setTransitioning(false)
    }, TRANSITION_MS)
  }

  const demo = DEMOS[activeIndex]
  const accent = ACCENT_CLASSES[demo.accent]

  return (
    <div className={cn('mx-auto w-full max-w-2xl', props.className)}>
      <div
        className={cn(
          'overflow-hidden rounded-2xl border backdrop-blur-sm',
          'border-border/60 bg-white/95 shadow-[0_20px_50px_-25px_rgba(15,23,42,0.18)]',
          'dark:border-white/[0.06] dark:bg-[#0b0f17]/95 dark:shadow-[0_20px_60px_-25px_rgba(0,0,0,0.7)]'
        )}
      >
        <div
          className={cn(
            'flex items-center gap-1 border-b px-2 sm:gap-1.5 sm:px-3',
            'border-border/50 dark:border-white/[0.05]'
          )}
        >
          {DEMOS.map((item, index) => {
            const tone = ACCENT_CLASSES[item.accent]
            const isActive = index === activeIndex
            return (
              <button
                key={item.id}
                onClick={() => handleSelect(index)}
                className={cn(
                  'relative -mb-px flex items-center gap-1.5 border-b-2 px-2.5 py-2.5 text-[11px] font-medium tracking-wide transition-colors sm:px-3 sm:text-xs',
                  isActive
                    ? `${tone.activeBorder} ${tone.activeText}`
                    : 'border-transparent text-foreground/40 hover:text-foreground/70'
                )}
              >
                {t(item.label)}
              </button>
            )
          })}
          <div className='ml-auto flex items-center gap-2 pr-2 sm:pr-3'>
            <span className='inline-block size-1.5 rounded-full bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.45)]' />
            <span className='text-foreground/40 font-mono text-[10px] tracking-wider uppercase'>
              200 ok
            </span>
          </div>
        </div>

        <div
          className={cn(
            'flex items-center gap-2.5 border-b px-5 py-3',
            'border-border/40 dark:border-white/[0.04]'
          )}
        >
          <span
            className={cn(
              'rounded-md px-1.5 py-0.5 font-mono text-[10px] font-semibold tracking-wider uppercase',
              accent.badge
            )}
          >
            {t(demo.badge)}
          </span>
          <code
            className={cn(
              'text-foreground/75 truncate font-mono text-[12.5px] transition-opacity duration-200',
              transitioning ? 'opacity-0' : 'opacity-100'
            )}
          >
            {t(demo.title)}
          </code>
        </div>

        <div className='grid h-[400px] grid-rows-[235px_minmax(0,1fr)] font-mono text-[12.5px] leading-[1.55]'>
          <RequestBlock demo={demo} transitioning={transitioning} />
          <ResponseBlock demo={demo} transitioning={transitioning} />
        </div>

        <div
          className={cn(
            'flex items-center justify-between border-t px-5 py-2.5',
            'border-border/40 bg-muted/30 dark:border-white/[0.05] dark:bg-white/[0.02]'
          )}
        >
          <div className='text-foreground/40 flex items-center gap-3 text-[10px] tabular-nums'>
            <span className='flex items-center gap-1'>
              <span className='font-mono'>{demo.latency}</span>
              <span className='tracking-wider uppercase'>ms</span>
            </span>
            <span className='bg-foreground/15 size-1 rounded-full' />
            <span className='flex items-center gap-1'>
              <span className='font-mono'>{demo.confidence}</span>
              <span className='tracking-wider uppercase'>{t('score')}</span>
            </span>
            <span className='bg-foreground/15 size-1 rounded-full' />
            <span className='flex items-center gap-1'>
              <span className='tracking-wider uppercase'>{t('workflow')}</span>
              <span className='font-mono'>print-api</span>
            </span>
          </div>
          <span className='text-foreground/30 font-mono text-[10px] tracking-wider uppercase'>
            {t('preview · structured output')}
          </span>
        </div>
      </div>
    </div>
  )
}

function RequestBlock(props: { demo: DemoConfig; transitioning: boolean }) {
  const { t } = useTranslation()

  return (
    <div className='relative px-5 py-4'>
      <SectionLabel>{t('Request')}</SectionLabel>
      <div
        className={cn(
          'mt-2 transition-opacity duration-200',
          props.transitioning ? 'opacity-0' : 'opacity-100'
        )}
      >
        {props.demo.request.map((line) => (
          <CodeLine key={line}>{renderJsonLine(line)}</CodeLine>
        ))}
      </div>
    </div>
  )
}

function ResponseBlock(props: { demo: DemoConfig; transitioning: boolean }) {
  const { t } = useTranslation()

  return (
    <div
      className={cn(
        'relative border-t px-5 py-4',
        'border-border/40 bg-muted/20 dark:border-white/[0.05] dark:bg-white/[0.015]'
      )}
    >
      <SectionLabel>{t('Response')}</SectionLabel>
      <div
        className={cn(
          'mt-2 transition-opacity duration-200',
          props.transitioning ? 'opacity-0' : 'opacity-100'
        )}
      >
        {props.demo.response.map((line) => (
          <CodeLine key={line}>{renderJsonLine(line)}</CodeLine>
        ))}
      </div>
    </div>
  )
}

function SectionLabel(props: { children: ReactNode }) {
  return (
    <span className='text-foreground/30 font-sans text-[10px] font-semibold tracking-[0.18em] uppercase'>
      {props.children}
    </span>
  )
}

function renderJsonLine(line: string): ReactNode {
  const trimmed = line.trim()
  if (!trimmed) return <Muted> </Muted>

  const isString = trimmed.startsWith('"') && trimmed.endsWith('"')
  if (isString) {
    return <StringText>{line}</StringText>
  }

  if (line.includes(':')) {
    const [key, ...rest] = line.split(':')
    return (
      <>
        <Key>{`${key}:`}</Key>
        <Muted>{rest.length > 0 ? rest.join(':') : ''}</Muted>
      </>
    )
  }

  if (line === '{' || line === '}' || line === '},') {
    return <Muted>{line}</Muted>
  }

  return <Muted>{line}</Muted>
}

function CodeLine(props: { children: ReactNode }) {
  return <div className='break-words whitespace-pre-wrap'>{props.children}</div>
}

function Key(props: { children: ReactNode }) {
  return <span className='text-sky-700 dark:text-sky-300'>{props.children}</span>
}

function StringText(props: { children: ReactNode }) {
  return <span className='text-emerald-600 dark:text-emerald-400'>{props.children}</span>
}

function Muted(props: { children: ReactNode }) {
  return <span className='text-foreground/50'>{props.children}</span>
}

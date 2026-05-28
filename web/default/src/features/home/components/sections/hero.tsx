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
import { Link } from '@tanstack/react-router'
import { ArrowRight, BookOpen, Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useStatus } from '@/hooks/use-status'
import { Button } from '@/components/ui/button'
import { BENEFIT_ITEMS, HERO_STAT_ITEMS, USE_CASES } from '../../constants'
import { HeroTerminalDemo } from '../hero-terminal-demo'

interface HeroProps {
  className?: string
  isAuthenticated?: boolean
}

export function Hero(props: HeroProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const docsUrl =
    (status?.docs_link as string | undefined) || 'https://docs.newapi.pro'

  const renderDocsButton = () => {
    const isExternal = docsUrl.startsWith('http')
    const className =
      'group border-border/50 hover:border-border hover:bg-muted/50 inline-flex h-11 items-center gap-1.5 rounded-lg px-5 text-sm font-medium'

    if (isExternal) {
      return (
        <Button
          variant='outline'
          className={className}
          render={
            <a href={docsUrl} target='_blank' rel='noopener noreferrer' />
          }
        >
          <BookOpen className='text-muted-foreground/80 group-hover:text-foreground size-4 transition-colors duration-200' />
          <span>{t('Docs')}</span>
        </Button>
      )
    }

    return (
      <Button variant='outline' className={className} render={<Link to={docsUrl} />}>
        <BookOpen className='text-muted-foreground/80 group-hover:text-foreground size-4 transition-colors duration-200' />
        <span>{t('Docs')}</span>
      </Button>
    )
  }

  return (
    <section className='relative z-10 overflow-hidden px-6 pt-24 pb-16 md:pt-32 md:pb-24 lg:pt-36 lg:pb-28'>
      <div
        aria-hidden
        className='pointer-events-none absolute inset-0 -z-10 opacity-30 dark:opacity-[0.14]'
        style={{
          background: [
            'radial-gradient(ellipse 60% 50% at 20% 20%, oklch(0.78 0.17 35 / 70%) 0%, transparent 70%)',
            'radial-gradient(ellipse 50% 40% at 80% 15%, oklch(0.72 0.18 320 / 55%) 0%, transparent 70%)',
            'radial-gradient(ellipse 42% 35% at 45% 82%, oklch(0.74 0.14 80 / 35%) 0%, transparent 70%)',
          ].join(', '),
        }}
      />
      <div
        aria-hidden
        className='absolute inset-0 -z-10 bg-[linear-gradient(to_right,var(--border)_1px,transparent_1px),linear-gradient(to_bottom,var(--border)_1px,transparent_1px)] [mask-image:radial-gradient(ellipse_62%_52%_at_50%_30%,black_20%,transparent_100%)] bg-[size:4rem_4rem] opacity-[0.08]'
      />

      <div className='mx-auto grid max-w-6xl grid-cols-1 items-start gap-12 lg:grid-cols-12 lg:gap-8'>
        <div className='flex flex-col items-start text-left lg:col-span-6'>
          <div
            className='landing-animate-fade-up mb-5 inline-flex items-center gap-1.5 rounded-full border border-orange-500/20 bg-orange-500/5 px-3 py-1.5 text-[11px] font-medium text-orange-600 opacity-0 shadow-xs dark:border-orange-400/20 dark:bg-orange-400/5 dark:text-orange-400'
            style={{ animationDelay: '0ms' }}
          >
            <span className='relative flex size-1.5'>
              <span className='absolute inline-flex h-full w-full animate-ping rounded-full bg-orange-400 opacity-75' />
              <span className='relative inline-flex size-1.5 rounded-full bg-orange-500 dark:bg-orange-400' />
            </span>
            <span>{t('Tuyao API · Focused AI Print Extraction')}</span>
          </div>

          <h1
            className='landing-animate-fade-up text-[clamp(2.25rem,4.5vw,3.35rem)] leading-[1.15] font-bold tracking-tight opacity-0'
            style={{ animationDelay: '60ms' }}
          >
            {t('AI print extraction')}
            <br />
            <span className='bg-gradient-to-r from-orange-400 via-pink-400 to-violet-500 bg-clip-text text-transparent'>
              {t('built for pattern production')}
            </span>
          </h1>
          <p
            className='landing-animate-fade-up text-muted-foreground/80 mt-5 max-w-xl text-base leading-relaxed opacity-0 md:text-[15px]'
            style={{ animationDelay: '120ms' }}
          >
            {t(
              'Tuyao API focuses on AI print extraction, turning fabric photos, sketches, and design references into clean pattern assets for review, production, and API workflows.'
            )}
          </p>

          <div
            className='landing-animate-fade-up mt-8 flex flex-wrap items-center gap-3 opacity-0'
            style={{ animationDelay: '180ms' }}
          >
            {props.isAuthenticated ? (
              <>
                <Button
                  className='group h-11 rounded-lg px-5 text-sm font-medium'
                  render={<Link to='/dashboard' />}
                >
                  {t('Go to Dashboard')}
                  <ArrowRight className='ml-1.5 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
                </Button>
                {renderDocsButton()}
              </>
            ) : (
              <>
                <Button
                  className='group h-11 rounded-lg px-5 text-sm font-medium'
                  render={<Link to='/sign-up' />}
                >
                  {t('Start extracting prints')}
                  <ArrowRight className='ml-1.5 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
                </Button>
                <Button
                  variant='outline'
                  className='border-border/50 hover:border-border hover:bg-muted/50 h-11 rounded-lg px-5 text-sm font-medium'
                  render={<Link to='/pricing' />}
                >
                  {t('View Pricing')}
                </Button>
                {renderDocsButton()}
              </>
            )}
          </div>

          <div
            className='landing-animate-fade-up mt-10 w-full max-w-xl opacity-0'
            style={{ animationDelay: '240ms' }}
          >
            <div className='mb-4 flex flex-col gap-1'>
              <span className='text-muted-foreground/50 text-[10px] font-bold tracking-[0.15em] uppercase'>
                {t('For print-driven workflows')}
              </span>
              <p className='text-muted-foreground/60 text-xs leading-relaxed'>
                {t('From image references to clean extraction results for design and production teams.')}
              </p>
            </div>
            <div className='flex flex-wrap items-center gap-2.5'>
              {USE_CASES.map((item) => (
                <span
                  key={item}
                  className='border-border/40 bg-muted/15 text-foreground/75 hover:border-border hover:bg-muted/30 inline-flex items-center gap-2 rounded-full border px-4 py-2 text-xs font-medium shadow-[0_1px_2.5px_rgba(0,0,0,0.01)] backdrop-blur-xs transition-colors duration-300'
                >
                  <Sparkles className='size-3.5 text-orange-500/80' />
                  {t(item)}
                </span>
              ))}
            </div>
          </div>

          <div
            className='landing-animate-fade-up mt-8 grid w-full max-w-xl gap-3 opacity-0 sm:grid-cols-2'
            style={{ animationDelay: '300ms' }}
          >
            {BENEFIT_ITEMS.map((item) => (
              <div
                key={item}
                className='border-border/40 bg-background/45 text-muted-foreground rounded-xl border px-4 py-3 text-xs shadow-xs backdrop-blur-sm'
              >
                {t(item)}
              </div>
            ))}
          </div>
        </div>

        <div
          className='landing-animate-fade-up flex w-full justify-center opacity-0 lg:col-span-6'
          style={{ animationDelay: '360ms' }}
        >
          <div className='w-full'>
            <HeroTerminalDemo className='mt-8 lg:mt-0' />
            <div className='mt-4 grid grid-cols-3 gap-3'>
              {HERO_STAT_ITEMS.map((item) => (
                <div
                  key={item.label}
                  className='border-border/40 bg-background/60 rounded-xl border px-3 py-3 text-center shadow-xs backdrop-blur-sm'
                >
                  <div className='text-sm font-bold tracking-tight md:text-base'>
                    {item.value}
                    {'suffix' in item && item.suffix ? item.suffix : ''}
                  </div>
                  <div className='text-muted-foreground mt-1 text-[10px] leading-snug'>
                    {t(item.label)}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}

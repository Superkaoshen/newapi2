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
import { ArrowUpRight, LayoutGrid, ScanText, Workflow } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { AnimateInView } from '@/components/animate-in-view'
import { WORKFLOW_STEPS } from '../../constants'

const ICONS = [
  <LayoutGrid className='size-6' strokeWidth={1.5} key='layout' />,
  <ScanText className='size-6' strokeWidth={1.5} key='scan' />,
  <Workflow className='size-6' strokeWidth={1.5} key='workflow' />,
]

export function HowItWorks() {
  const { t } = useTranslation()

  return (
    <section className='border-border/40 relative z-10 border-t px-6 py-24 md:py-32'>
      <div className='mx-auto max-w-6xl'>
        <AnimateInView className='mb-16 text-center md:mb-20'>
          <p className='text-muted-foreground mb-3 text-xs font-medium tracking-widest uppercase'>
            {t('How it works')}
          </p>
          <h2 className='text-2xl font-bold tracking-tight md:text-3xl'>
            {t('From upload to production-ready print output')}
          </h2>
        </AnimateInView>

        <div className='grid gap-8 md:grid-cols-3 md:gap-12'>
          {WORKFLOW_STEPS.map((step, index) => (
            <AnimateInView
              key={step.title}
              delay={index * 150}
              animation='fade-up'
              className='relative flex flex-col items-center text-center'
            >
              <div className='relative mb-6'>
                <div className='text-muted-foreground border-border/50 bg-muted/30 flex size-16 items-center justify-center rounded-2xl border transition-colors'>
                  {ICONS[index]}
                </div>
                <div className='bg-foreground text-background absolute -top-2 -right-2 flex size-6 items-center justify-center rounded-full text-xs font-bold'>
                  {index + 1}
                </div>
              </div>
              <h3 className='mb-2 text-base font-semibold'>{t(step.title)}</h3>
              <p className='text-muted-foreground max-w-[240px] text-sm leading-relaxed'>
                {t(step.description)}
              </p>
              {index < WORKFLOW_STEPS.length - 1 ? (
                <ArrowUpRight className='text-muted-foreground/40 mt-5 size-4 md:mt-6' />
              ) : null}
            </AnimateInView>
          ))}
        </div>
      </div>
    </section>
  )
}

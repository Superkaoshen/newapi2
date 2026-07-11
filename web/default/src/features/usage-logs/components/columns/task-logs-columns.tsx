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
/* eslint-disable react-refresh/only-export-components */
import { useState, useMemo } from 'react'
import type { ColumnDef } from '@tanstack/react-table'
import { ImageIcon, Music } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { getUserAvatarFallback, getUserAvatarStyle } from '@/lib/avatar'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { DataTableColumnHeader } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { TASK_ACTIONS, TASK_STATUS } from '../../constants'
import { taskActionMapper, taskStatusMapper } from '../../lib/mappers'
import type { TaskLog } from '../../types'
import {
  AudioPreviewDialog,
  type AudioClip,
} from '../dialogs/audio-preview-dialog'
import { FailReasonDialog } from '../dialogs/fail-reason-dialog'
import { ImageDialog } from '../dialogs/image-dialog'
import { useUsageLogsContext } from '../usage-logs-provider'
import {
  createDurationColumn,
  createChannelColumn,
  createProgressColumn,
} from './column-helpers'

const IMAGE_TASK_PLATFORMS = new Set(['58', '59'])
const VIDEO_TASK_PLATFORMS = new Set([
  'kling',
  'runway',
  'luma',
  'viggle',
  '50',
  '51',
  '52',
  '54',
  '55',
])

const TASK_PLATFORM_LABELS: Record<string, string> = {
  '24': 'Gemini',
  '50': 'Kling',
  '51': 'Jimeng',
  '52': 'Vidu',
  '54': 'DoubaoVideo',
  '55': 'Sora',
  '58': 'Mihuifang',
  // Firefly is an internal upstream implementation of the Mihuifang protocol.
  '59': 'Mihuifang',
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value))
}

function trimString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function parseJSONValue(value: unknown): unknown {
  if (typeof value !== 'string') return value
  try {
    return JSON.parse(value)
  } catch {
    return value
  }
}

function parseTaskData(data: unknown): unknown[] {
  const parsed = parseJSONValue(data)
  return Array.isArray(parsed) ? parsed : []
}

function isLikelyImageURL(url: string): boolean {
  return /\.(avif|bmp|gif|heic|heif|jpe?g|png|svg|tiff?|webp)(?:[?#].*)?$/i.test(
    url
  )
}

function isLikelyVideoURL(url: string): boolean {
  return /\.(m3u8|m4v|mov|mp4|mpeg|mpg|webm)(?:[?#].*)?$/i.test(url)
}

function collectStringURLs(
  value: unknown,
  urls: string[],
  shouldInclude: (url: string) => boolean = () => true
) {
  if (typeof value === 'string') {
    const url = value.trim()
    if (url.startsWith('http') && shouldInclude(url)) urls.push(url)
  } else if (Array.isArray(value)) {
    value.forEach((item) => collectStringURLs(item, urls, shouldInclude))
  }
}

function collectImageURLsFromResult(result: unknown): string[] {
  if (!isRecord(result)) return []
  const urls: string[] = []

  collectStringURLs(result.image_url, urls)
  collectStringURLs(result.image_urls, urls)
  collectStringURLs(result.url, urls, isLikelyImageURL)

  if (Array.isArray(result.items)) {
    result.items.forEach((item) => {
      if (!isRecord(item)) return
      const itemType = trimString(item.type).toLowerCase()
      collectStringURLs(
        item.url,
        urls,
        (url) => itemType === 'image' || isLikelyImageURL(url)
      )
    })
  }

  return urls.filter((url, index) => urls.indexOf(url) === index)
}

function collectImageURLsFromTaskData(data: unknown): string[] {
  const parsed = parseJSONValue(data)
  if (!isRecord(parsed)) return []

  const urls = [
    ...collectImageURLsFromResult(parsed.result),
    ...collectImageURLsFromResult(parsed),
  ]

  return urls.filter((url, index) => urls.indexOf(url) === index)
}

function getTaskResultURL(log: TaskLog): string {
  return trimString(log.result_url) || trimString(log.fail_reason)
}

function getTaskModelName(log: TaskLog): string {
  const properties = parseJSONValue(log.properties)
  if (!isRecord(properties)) return ''
  return (
    trimString(properties.origin_model_name) ||
    trimString(properties.upstream_model_name) ||
    trimString(properties.model)
  )
}

function isImageTaskContext(log: TaskLog): boolean {
  if (IMAGE_TASK_PLATFORMS.has(String(log.platform))) return true

  const modelName = getTaskModelName(log).toLowerCase()
  return (
    modelName.includes('image') ||
    modelName.includes('nano-banana') ||
    modelName.includes('banana')
  )
}

function getTaskImageURL(log: TaskLog): string {
  const dataImageURL = collectImageURLsFromTaskData(log.data)[0]
  if (dataImageURL) return dataImageURL

  const resultURL = getTaskResultURL(log)
  if (!resultURL) return ''
  if (isLikelyImageURL(resultURL)) return resultURL
  if (isImageTaskContext(log) && !isLikelyVideoURL(resultURL)) return resultURL
  return ''
}

function isVideoTaskAction(action: string): boolean {
  return (
    action === TASK_ACTIONS.GENERATE ||
    action === TASK_ACTIONS.TEXT_GENERATE ||
    action === TASK_ACTIONS.FIRST_TAIL_GENERATE ||
    action === TASK_ACTIONS.REFERENCE_GENERATE ||
    action === TASK_ACTIONS.REMIX_GENERATE
  )
}

function isVideoTaskContext(log: TaskLog): boolean {
  if (!isVideoTaskAction(log.action)) return false
  if (getTaskImageURL(log)) return false
  if (VIDEO_TASK_PLATFORMS.has(String(log.platform))) return true

  const resultURL = getTaskResultURL(log)
  return isLikelyVideoURL(resultURL)
}

function getTaskActionLabel(log: TaskLog): string {
  if (getTaskImageURL(log) || isImageTaskContext(log)) return 'Image Generation'
  return taskActionMapper.getLabel(log.action)
}

function getTaskPlatformLabel(platform: string): string {
  return TASK_PLATFORM_LABELS[platform] || platform
}

function AudioPreviewCell({ log }: { log: TaskLog }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const clips = useMemo(() => {
    const data = parseTaskData(log.data)
    return data.filter(
      (c) =>
        c && typeof c === 'object' && (c as Record<string, unknown>).audio_url
    )
  }, [log.data])

  if (clips.length === 0) return null

  return (
    <>
      <button
        type='button'
        className='group flex items-center gap-1 text-left text-xs'
        onClick={() => setOpen(true)}
      >
        <Music className='text-muted-foreground size-3' />
        <span className='text-foreground leading-snug group-hover:underline'>
          {t('Click to preview audio')}
        </span>
      </button>
      <AudioPreviewDialog
        open={open}
        onOpenChange={setOpen}
        clips={clips as AudioClip[]}
      />
    </>
  )
}

function ImagePreviewCell({
  imageUrl,
  taskId,
}: {
  imageUrl: string
  taskId: string
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  return (
    <>
      <button
        type='button'
        className='group flex items-center gap-1 text-left text-xs'
        onClick={() => setOpen(true)}
        title={t('Click to view image')}
      >
        <ImageIcon className='text-muted-foreground size-3' />
        <span className='text-foreground leading-snug group-hover:underline'>
          {t('Click to view image')}
        </span>
      </button>
      <ImageDialog
        imageUrl={imageUrl}
        taskId={taskId}
        open={open}
        onOpenChange={setOpen}
      />
    </>
  )
}

export function useTaskLogsColumns(isAdmin: boolean): ColumnDef<TaskLog>[] {
  const { t } = useTranslation()
  const columns: ColumnDef<TaskLog>[] = [
    {
      accessorKey: 'submit_time',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Submit Time')} />
      ),
      cell: ({ row }) => {
        const log = row.original
        const submitTime = row.getValue('submit_time') as number

        return (
          <div className='flex flex-col gap-0.5'>
            <span className='font-mono text-xs tabular-nums'>
              {formatTimestampToDate(submitTime, 'seconds')}
            </span>
            {log.finish_time ? (
              <span className='text-muted-foreground/60 font-mono text-[11px] tabular-nums'>
                {formatTimestampToDate(log.finish_time, 'seconds')}
              </span>
            ) : (
              <span className='text-muted-foreground/50 text-[11px]'>-</span>
            )}
          </div>
        )
      },
      meta: { label: t('Submit Time') },
    },
  ]

  if (isAdmin) {
    columns.push(createChannelColumn<TaskLog>({ headerLabel: t('Channel') }), {
      id: 'user',
      accessorFn: (row) => row.username || row.user_id,
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('User')} />
      ),
      cell: function UserCell({ row }) {
        const { sensitiveVisible, setSelectedUserId, setUserInfoDialogOpen } =
          useUsageLogsContext()
        const log = row.original
        const displayName = log.username || String(log.user_id || '?')

        return (
          <button
            type='button'
            className='flex items-center gap-1.5 text-left'
            onClick={(e) => {
              e.stopPropagation()
              setSelectedUserId(log.user_id)
              setUserInfoDialogOpen(true)
            }}
          >
            <Avatar className='ring-border/60 size-6 ring-1 max-sm:hidden'>
              <AvatarFallback
                className={cn(
                  'text-[11px] font-semibold',
                  !sensitiveVisible && 'bg-muted text-muted-foreground'
                )}
                style={
                  sensitiveVisible ? getUserAvatarStyle(displayName) : undefined
                }
              >
                {sensitiveVisible ? getUserAvatarFallback(displayName) : '•'}
              </AvatarFallback>
            </Avatar>
            <span className='text-muted-foreground truncate text-sm hover:underline'>
              {sensitiveVisible ? displayName : '••••'}
            </span>
          </button>
        )
      },
      meta: { label: t('User') },
    })
  }

  columns.push(
    {
      accessorKey: 'task_id',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Task ID')} />
      ),
      cell: ({ row }) => {
        const log = row.original
        const taskId = row.getValue('task_id') as string
        if (!taskId) {
          return <span className='text-muted-foreground/60 text-xs'>-</span>
        }
        return (
          <div className='flex max-w-[170px] flex-col gap-0.5'>
            <StatusBadge
              label={taskId}
              autoColor={taskId}
              size='sm'
              className='border-border/60 bg-muted/30 max-w-full truncate rounded-md border px-1.5 py-0.5 font-mono'
            />
            <span className='text-muted-foreground/60 truncate text-[11px]'>
              {t(getTaskPlatformLabel(log.platform))} ·{' '}
              {t(getTaskActionLabel(log))}
            </span>
          </div>
        )
      },
      meta: { label: t('Task ID'), mobileTitle: true },
    },
    createDurationColumn<TaskLog>({
      submitTimeKey: 'submit_time',
      finishTimeKey: 'finish_time',
      unit: 'seconds',
      headerLabel: t('Duration'),
      warningThresholdSec: 300,
    }),
    {
      accessorKey: 'status',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Status')} />
      ),
      cell: ({ row }) => {
        const status = row.getValue('status') as string
        return (
          <StatusBadge
            label={t(taskStatusMapper.getLabel(status, status || 'Submitting'))}
            variant={taskStatusMapper.getVariant(status)}
            size='sm'
            copyable={false}
          />
        )
      },
      meta: { label: t('Status') },
    },
    createProgressColumn<TaskLog>({ headerLabel: t('Progress') }),
    {
      accessorKey: 'fail_reason',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Details')} />
      ),
      cell: function DetailsCell({ row }) {
        const log = row.original
        const failReason = row.getValue('fail_reason') as string
        const status = log.status
        const [dialogOpen, setDialogOpen] = useState(false)
        const imageURL = getTaskImageURL(log)

        const isSunoSuccess =
          log.platform === 'suno' && status === TASK_STATUS.SUCCESS
        if (isSunoSuccess) {
          const data = parseTaskData(log.data)
          if (
            data.some(
              (c) =>
                c &&
                typeof c === 'object' &&
                (c as Record<string, unknown>).audio_url
            )
          ) {
            return <AudioPreviewCell log={log} />
          }
        }

        const isSuccess = status === TASK_STATUS.SUCCESS

        if (isSuccess && imageURL) {
          return <ImagePreviewCell imageUrl={imageURL} taskId={log.task_id} />
        }

        if (isSuccess && isVideoTaskContext(log)) {
          const videoUrl = `/v1/videos/${log.task_id}/content`
          return (
            <a
              href={videoUrl}
              target='_blank'
              rel='noopener noreferrer'
              className='text-foreground text-xs hover:underline'
            >
              {t('Click to preview video')}
            </a>
          )
        }

        if (!failReason) {
          return <span className='text-muted-foreground/60 text-xs'>-</span>
        }

        return (
          <>
            <button
              type='button'
              className='group flex max-w-[200px] items-center gap-1 text-left text-xs'
              onClick={() => setDialogOpen(true)}
              title={t('Click to view full error message')}
            >
              <span className='truncate leading-snug text-red-600 group-hover:underline dark:text-red-400'>
                {failReason}
              </span>
            </button>
            <FailReasonDialog
              failReason={failReason}
              open={dialogOpen}
              onOpenChange={setDialogOpen}
            />
          </>
        )
      },
      meta: { label: t('Details') },
      size: 200,
      maxSize: 220,
    }
  )

  return columns
}

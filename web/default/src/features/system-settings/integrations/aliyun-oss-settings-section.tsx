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
import * as z from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { removeTrailingSlash } from './utils'

const isBlankOrURL = (value: string) => {
  const trimmed = value.trim()
  return trimmed === '' || /^https?:\/\//.test(trimmed)
}

const isBlankOrOssEndpoint = (value: string) => {
  const trimmed = value.trim()
  if (trimmed === '') return true
  if (trimmed.includes('://')) return /^https?:\/\/[^/\s]+\/?$/.test(trimmed)
  return /^[a-zA-Z0-9.-]+$/.test(trimmed)
}

const createAliyunOssSchema = (t: (key: string) => string) =>
  z.object({
    AliyunOssEnabled: z.boolean(),
    AliyunOssEndpoint: z
      .string()
      .refine(
        isBlankOrOssEndpoint,
        t('Provide a valid OSS endpoint host or URL')
      ),
    AliyunOssBucket: z.string(),
    AliyunOssAccessKeyId: z.string(),
    AliyunOssAccessKeySecret: z.string(),
    AliyunOssPathPrefix: z.string(),
    AliyunOssPublicBaseUrl: z
      .string()
      .refine(
        isBlankOrURL,
        t('Provide a valid URL starting with http:// or https://')
      ),
    AliyunOssUploadTimeoutSeconds: z.coerce.number().int().min(1).max(300),
  })

export type AliyunOssFormValues = z.infer<
  ReturnType<typeof createAliyunOssSchema>
>

type AliyunOssSettingsSectionProps = {
  defaultValues: AliyunOssFormValues
}

function normalizeEndpoint(value: string) {
  const trimmed = value.trim()
  if (!trimmed.includes('://')) return trimmed
  return removeTrailingSlash(trimmed)
}

export function AliyunOssSettingsSection({
  defaultValues,
}: AliyunOssSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const aliyunOssSchema = createAliyunOssSchema(t)

  const form = useForm<AliyunOssFormValues>({
    resolver: zodResolver(aliyunOssSchema),
    defaultValues,
  })

  useResetForm(form, defaultValues)

  const onSubmit = async (values: AliyunOssFormValues) => {
    const sanitized: AliyunOssFormValues = {
      AliyunOssEnabled: values.AliyunOssEnabled,
      AliyunOssEndpoint: normalizeEndpoint(values.AliyunOssEndpoint),
      AliyunOssBucket: values.AliyunOssBucket.trim(),
      AliyunOssAccessKeyId: values.AliyunOssAccessKeyId.trim(),
      AliyunOssAccessKeySecret: values.AliyunOssAccessKeySecret.trim(),
      AliyunOssPathPrefix: values.AliyunOssPathPrefix.trim(),
      AliyunOssPublicBaseUrl: removeTrailingSlash(
        values.AliyunOssPublicBaseUrl.trim()
      ),
      AliyunOssUploadTimeoutSeconds: values.AliyunOssUploadTimeoutSeconds,
    }

    const initial: AliyunOssFormValues = {
      AliyunOssEnabled: defaultValues.AliyunOssEnabled,
      AliyunOssEndpoint: normalizeEndpoint(defaultValues.AliyunOssEndpoint),
      AliyunOssBucket: defaultValues.AliyunOssBucket.trim(),
      AliyunOssAccessKeyId: defaultValues.AliyunOssAccessKeyId.trim(),
      AliyunOssAccessKeySecret: defaultValues.AliyunOssAccessKeySecret.trim(),
      AliyunOssPathPrefix: defaultValues.AliyunOssPathPrefix.trim(),
      AliyunOssPublicBaseUrl: removeTrailingSlash(
        defaultValues.AliyunOssPublicBaseUrl.trim()
      ),
      AliyunOssUploadTimeoutSeconds:
        defaultValues.AliyunOssUploadTimeoutSeconds,
    }

    const updates: Array<{ key: string; value: string | boolean | number }> = []

    if (sanitized.AliyunOssEnabled !== initial.AliyunOssEnabled) {
      updates.push({
        key: 'AliyunOssEnabled',
        value: sanitized.AliyunOssEnabled,
      })
    }

    if (sanitized.AliyunOssEndpoint !== initial.AliyunOssEndpoint) {
      updates.push({
        key: 'AliyunOssEndpoint',
        value: sanitized.AliyunOssEndpoint,
      })
    }

    if (sanitized.AliyunOssBucket !== initial.AliyunOssBucket) {
      updates.push({ key: 'AliyunOssBucket', value: sanitized.AliyunOssBucket })
    }

    if (
      sanitized.AliyunOssAccessKeyId &&
      sanitized.AliyunOssAccessKeyId !== initial.AliyunOssAccessKeyId
    ) {
      updates.push({
        key: 'AliyunOssAccessKeyId',
        value: sanitized.AliyunOssAccessKeyId,
      })
    }

    if (sanitized.AliyunOssAccessKeySecret) {
      updates.push({
        key: 'AliyunOssAccessKeySecret',
        value: sanitized.AliyunOssAccessKeySecret,
      })
    }

    if (sanitized.AliyunOssPathPrefix !== initial.AliyunOssPathPrefix) {
      updates.push({
        key: 'AliyunOssPathPrefix',
        value: sanitized.AliyunOssPathPrefix,
      })
    }

    if (sanitized.AliyunOssPublicBaseUrl !== initial.AliyunOssPublicBaseUrl) {
      updates.push({
        key: 'AliyunOssPublicBaseUrl',
        value: sanitized.AliyunOssPublicBaseUrl,
      })
    }

    if (
      sanitized.AliyunOssUploadTimeoutSeconds !==
      initial.AliyunOssUploadTimeoutSeconds
    ) {
      updates.push({
        key: 'AliyunOssUploadTimeoutSeconds',
        value: sanitized.AliyunOssUploadTimeoutSeconds,
      })
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
  }

  return (
    <SettingsSection title={t('Aliyun OSS')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save Aliyun OSS settings'
          />

          <Alert>
            <AlertDescription>
              {t(
                'Generated image results are copied to Aliyun OSS when enabled. Use an OSS region close to this server to avoid slow cross-region uploads.'
              )}
            </AlertDescription>
          </Alert>

          <FormField
            control={form.control}
            name='AliyunOssEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable Aliyun OSS')}</FormLabel>
                  <FormDescription>
                    {t(
                      'When enabled, generated image and file results are stored in your OSS bucket before being returned.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <FormField
            control={form.control}
            name='AliyunOssEndpoint'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('OSS Endpoint')}</FormLabel>
                <FormControl>
                  <Input
                    inputMode='url'
                    placeholder={t('oss-cn-hangzhou.aliyuncs.com')}
                    autoComplete='off'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Use the regional endpoint, for example oss-cn-hangzhou.aliyuncs.com.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='AliyunOssBucket'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Bucket')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('my-bucket')}
                    autoComplete='off'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t('The OSS bucket that stores generated assets.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='AliyunOssAccessKeyId'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('AccessKey ID')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('Enter AccessKey ID')}
                    autoComplete='off'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t('Leave blank to keep the existing AccessKey ID unchanged.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='AliyunOssAccessKeySecret'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('AccessKey Secret')}</FormLabel>
                <FormControl>
                  <Input
                    type='password'
                    placeholder={t('Enter new secret to update')}
                    autoComplete='new-password'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Leave blank to keep the existing AccessKey Secret unchanged.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='AliyunOssPathPrefix'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Path prefix')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('openai-images')}
                    autoComplete='off'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t('Objects are written under this prefix.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='AliyunOssPublicBaseUrl'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Public base URL')}</FormLabel>
                <FormControl>
                  <Input
                    type='url'
                    inputMode='url'
                    placeholder={t('https://cdn.example.com')}
                    autoComplete='off'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Optional. Use a custom domain or CDN URL for public access.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='AliyunOssUploadTimeoutSeconds'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Upload timeout seconds')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    inputMode='numeric'
                    min={1}
                    max={300}
                    {...field}
                    onChange={(event) =>
                      field.onChange(Number(event.target.value))
                    }
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Fail OSS uploads after this many seconds to prevent task log requests from hanging.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

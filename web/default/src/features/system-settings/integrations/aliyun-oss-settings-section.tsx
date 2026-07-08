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
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { removeTrailingSlash } from './utils'

const isBlankOrURL = (value: string) => {
  const trimmed = value.trim()
  return trimmed === '' || /^https?:\/\//.test(trimmed)
}

const isBlankOrStorageEndpoint = (value: string) => {
  const trimmed = value.trim()
  if (trimmed === '') return true
  if (trimmed.includes('://')) return /^https?:\/\/[^/\s]+\/?$/.test(trimmed)
  return /^[a-zA-Z0-9.-]+$/.test(trimmed)
}

const storageProviders = ['disabled', 'aliyun_oss', 'cloudflare_r2'] as const
const storageProviderSet = new Set<string>(storageProviders)

type ImageStorageProvider = (typeof storageProviders)[number]

const createObjectStorageSchema = (t: (key: string) => string) =>
  z.object({
    ImageStorageProvider: z.enum(storageProviders),
    AliyunOssEnabled: z.boolean(),
    AliyunOssEndpoint: z
      .string()
      .refine(
        isBlankOrStorageEndpoint,
        t('Provide a valid storage endpoint host or URL')
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
    R2Endpoint: z
      .string()
      .refine(
        isBlankOrStorageEndpoint,
        t('Provide a valid storage endpoint host or URL')
      ),
    R2Bucket: z.string(),
    R2AccessKeyId: z.string(),
    R2AccessKeySecret: z.string(),
    R2PathPrefix: z.string(),
    R2PublicBaseUrl: z
      .string()
      .refine(
        isBlankOrURL,
        t('Provide a valid URL starting with http:// or https://')
      ),
    R2Region: z.string(),
    R2UploadTimeoutSeconds: z.coerce.number().int().min(1).max(300),
  })

export type ObjectStorageFormValues = z.infer<
  ReturnType<typeof createObjectStorageSchema>
>

export type ObjectStorageSettingsValues = Omit<
  ObjectStorageFormValues,
  'ImageStorageProvider'
> & {
  ImageStorageProvider: ImageStorageProvider | ''
}

type AliyunOssSettingsSectionProps = {
  defaultValues: ObjectStorageSettingsValues
}

function normalizeEndpoint(value: string) {
  const trimmed = value.trim()
  if (!trimmed.includes('://')) return trimmed
  return removeTrailingSlash(trimmed)
}

function normalizeProvider(
  value: string,
  aliyunOssEnabled: boolean
): ImageStorageProvider {
  if (storageProviderSet.has(value)) {
    return value as ImageStorageProvider
  }
  return aliyunOssEnabled ? 'aliyun_oss' : 'disabled'
}

function sanitizeValues(
  values: ObjectStorageSettingsValues
): ObjectStorageFormValues {
  const provider = normalizeProvider(
    values.ImageStorageProvider,
    values.AliyunOssEnabled
  )

  return {
    ImageStorageProvider: provider,
    AliyunOssEnabled: provider === 'aliyun_oss',
    AliyunOssEndpoint: normalizeEndpoint(values.AliyunOssEndpoint),
    AliyunOssBucket: values.AliyunOssBucket.trim(),
    AliyunOssAccessKeyId: values.AliyunOssAccessKeyId.trim(),
    AliyunOssAccessKeySecret: values.AliyunOssAccessKeySecret.trim(),
    AliyunOssPathPrefix: values.AliyunOssPathPrefix.trim(),
    AliyunOssPublicBaseUrl: removeTrailingSlash(
      values.AliyunOssPublicBaseUrl.trim()
    ),
    AliyunOssUploadTimeoutSeconds: values.AliyunOssUploadTimeoutSeconds,
    R2Endpoint: normalizeEndpoint(values.R2Endpoint),
    R2Bucket: values.R2Bucket.trim(),
    R2AccessKeyId: values.R2AccessKeyId.trim(),
    R2AccessKeySecret: values.R2AccessKeySecret.trim(),
    R2PathPrefix: values.R2PathPrefix.trim(),
    R2PublicBaseUrl: removeTrailingSlash(values.R2PublicBaseUrl.trim()),
    R2Region: values.R2Region.trim() || 'auto',
    R2UploadTimeoutSeconds: values.R2UploadTimeoutSeconds,
  }
}

export function AliyunOssSettingsSection({
  defaultValues,
}: AliyunOssSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const objectStorageSchema = createObjectStorageSchema(t)
  const normalizedDefaultValues = sanitizeValues(defaultValues)

  const form = useForm<ObjectStorageFormValues>({
    resolver: zodResolver(objectStorageSchema),
    defaultValues: normalizedDefaultValues,
  })

  useResetForm(form, normalizedDefaultValues)

  const onSubmit = async (values: ObjectStorageFormValues) => {
    const sanitized = sanitizeValues(values)
    const initial = normalizedDefaultValues

    const updates: Array<{ key: string; value: string | boolean | number }> = []

    const changedKeys: Array<keyof ObjectStorageFormValues> = [
      'ImageStorageProvider',
      'AliyunOssEnabled',
      'AliyunOssEndpoint',
      'AliyunOssBucket',
      'AliyunOssPathPrefix',
      'AliyunOssPublicBaseUrl',
      'AliyunOssUploadTimeoutSeconds',
      'R2Endpoint',
      'R2Bucket',
      'R2PathPrefix',
      'R2PublicBaseUrl',
      'R2Region',
      'R2UploadTimeoutSeconds',
    ]

    for (const key of changedKeys) {
      if (sanitized[key] !== initial[key]) {
        updates.push({ key, value: sanitized[key] })
      }
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

    if (
      sanitized.R2AccessKeyId &&
      sanitized.R2AccessKeyId !== initial.R2AccessKeyId
    ) {
      updates.push({
        key: 'R2AccessKeyId',
        value: sanitized.R2AccessKeyId,
      })
    }

    if (sanitized.R2AccessKeySecret) {
      updates.push({
        key: 'R2AccessKeySecret',
        value: sanitized.R2AccessKeySecret,
      })
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
  }

  return (
    <SettingsSection title={t('Object Storage')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save object storage settings'
          />

          <Alert>
            <AlertDescription>
              {t(
                'Generated image and file results can be copied to the selected object storage provider before being returned. Use a storage region close to this server to avoid slow uploads.'
              )}
            </AlertDescription>
          </Alert>

          <FormField
            control={form.control}
            name='ImageStorageProvider'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Storage provider')}</FormLabel>
                <Select
                  items={[
                    {
                      value: 'disabled',
                      label: t('Disabled'),
                    },
                    {
                      value: 'aliyun_oss',
                      label: t('Aliyun OSS'),
                    },
                    {
                      value: 'cloudflare_r2',
                      label: t('Cloudflare R2'),
                    },
                  ]}
                  onValueChange={field.onChange}
                  value={field.value}
                >
                  <FormControl>
                    <SelectTrigger className='w-full'>
                      <SelectValue />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      <SelectItem value='disabled'>{t('Disabled')}</SelectItem>
                      <SelectItem value='aliyun_oss'>
                        {t('Aliyun OSS')}
                      </SelectItem>
                      <SelectItem value='cloudflare_r2'>
                        {t('Cloudflare R2')}
                      </SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <FormDescription>
                  {field.value === 'aliyun_oss'
                    ? t('Copy generated assets to Aliyun OSS')
                    : field.value === 'cloudflare_r2'
                      ? t('Copy generated assets to Cloudflare R2')
                      : t('Do not copy generated assets')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <div
            data-settings-form-span='full'
            className='flex min-w-0 flex-col gap-1'
          >
            <h3 className='text-sm font-medium'>{t('Aliyun OSS settings')}</h3>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Configure the Aliyun OSS bucket used when Aliyun OSS is selected.'
              )}
            </p>
          </div>

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
                    'Use the regional OSS endpoint, for example oss-cn-hangzhou.aliyuncs.com.'
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
                  {t('The bucket that stores generated assets.')}
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
                    'Fail object storage uploads after this many seconds to prevent task log requests from hanging.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <div
            data-settings-form-span='full'
            className='flex min-w-0 flex-col gap-1'
          >
            <h3 className='text-sm font-medium'>
              {t('Cloudflare R2 settings')}
            </h3>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Configure the Cloudflare R2 bucket used when Cloudflare R2 is selected.'
              )}
            </p>
          </div>

          <FormField
            control={form.control}
            name='R2Endpoint'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('R2 endpoint')}</FormLabel>
                <FormControl>
                  <Input
                    inputMode='url'
                    placeholder='<account-id>.r2.cloudflarestorage.com'
                    autoComplete='off'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Use the R2 S3 API endpoint, for example <account-id>.r2.cloudflarestorage.com.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='R2Bucket'
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
                  {t('The bucket that stores generated assets.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='R2AccessKeyId'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('R2 Access Key ID')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('Enter AccessKey ID')}
                    autoComplete='off'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Leave blank to keep the existing R2 Access Key ID unchanged.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='R2AccessKeySecret'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('R2 Secret Access Key')}</FormLabel>
                <FormControl>
                  <Input
                    type='password'
                    placeholder={t('Enter new secret to update')}
                    autoComplete='new-password'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t('Leave blank to keep the existing R2 secret unchanged.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='R2PathPrefix'
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
            name='R2PublicBaseUrl'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('R2 public base URL')}</FormLabel>
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
                    'Optional. Use an R2 public domain or CDN URL for public access.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='R2Region'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('R2 region')}</FormLabel>
                <FormControl>
                  <Input placeholder='auto' autoComplete='off' {...field} />
                </FormControl>
                <FormDescription>
                  {t(
                    'Use auto for Cloudflare R2 unless you use another S3-compatible region.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='R2UploadTimeoutSeconds'
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
                    'Fail object storage uploads after this many seconds to prevent task log requests from hanging.'
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

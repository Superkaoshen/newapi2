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

const createOssSchema = (t: (key: string) => string) =>
  z.object({
    AliyunOssEnabled: z.boolean(),
    AliyunOssEndpoint: z.string(),
    AliyunOssBucket: z.string(),
    AliyunOssAccessKeyId: z.string(),
    AliyunOssAccessKeySecret: z.string(),
    AliyunOssPathPrefix: z.string(),
    AliyunOssPublicBaseUrl: z.string().refine((value) => {
      const trimmed = value.trim()
      if (!trimmed) return true
      return /^https?:\/\//.test(trimmed)
    }, t('Provide a valid URL starting with http:// or https://')),
  })

type OssFormValues = z.infer<ReturnType<typeof createOssSchema>>

type AliyunOssSettingsSectionProps = {
  defaultValues: OssFormValues
}

export function AliyunOssSettingsSection({
  defaultValues,
}: AliyunOssSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const ossSchema = createOssSchema(t)

  const form = useForm<OssFormValues>({
    resolver: zodResolver(ossSchema),
    defaultValues,
  })

  useResetForm(form, defaultValues)

  const onSubmit = async (values: OssFormValues) => {
    const updates: Array<{ key: string; value: string | boolean }> = []

    if (values.AliyunOssEnabled !== defaultValues.AliyunOssEnabled) {
      updates.push({ key: 'AliyunOssEnabled', value: values.AliyunOssEnabled })
    }

    const fields: Array<{
      key: keyof Omit<OssFormValues, 'AliyunOssEnabled'>
      sanitize?: (v: string) => string
    }> = [
      { key: 'AliyunOssEndpoint', sanitize: removeTrailingSlash },
      { key: 'AliyunOssBucket' },
      { key: 'AliyunOssAccessKeyId' },
      { key: 'AliyunOssPathPrefix' },
      { key: 'AliyunOssPublicBaseUrl', sanitize: removeTrailingSlash },
    ]

    for (const { key, sanitize } of fields) {
      const rawValue = values[key] as string
      const sanitizedValue = sanitize ? sanitize(rawValue) : rawValue.trim()
      const initialValue = (defaultValues[key] as string) ?? ''
      if (sanitizedValue !== initialValue) {
        updates.push({ key: key as string, value: sanitizedValue })
      }
    }

    const secretValue = values.AliyunOssAccessKeySecret.trim()
    if (secretValue) {
      updates.push({ key: 'AliyunOssAccessKeySecret', value: secretValue })
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
  }

  const enabled = form.watch('AliyunOssEnabled')

  return (
    <SettingsSection title={t('Aliyun OSS')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save OSS settings'
          />
          <FormField
            control={form.control}
            name='AliyunOssEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable Aliyun OSS')}</FormLabel>
                  <FormDescription>
                    {t(
                      'When enabled, inline images returned by Gemini channel will be uploaded to OSS and replaced with public URLs.'
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

          {enabled && (
            <>
              <FormField
                control={form.control}
                name='AliyunOssEndpoint'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('OSS Endpoint')}</FormLabel>
                    <FormControl>
                      <Input
                        type='url'
                        inputMode='url'
                        placeholder={t('https://oss-cn-hangzhou.aliyuncs.com')}
                        autoComplete='off'
                        {...field}
                        onChange={(event) =>
                          field.onChange(event.target.value)
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Alibaba Cloud OSS endpoint, e.g. https://oss-cn-hangzhou.aliyuncs.com'
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
                    <FormLabel>{t('OSS Bucket')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('my-bucket-name')}
                        autoComplete='off'
                        {...field}
                        onChange={(event) =>
                          field.onChange(event.target.value)
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t('The OSS bucket name where images will be stored.')}
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
                    <FormLabel>{t('Access Key ID')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('LTAI...')}
                        autoComplete='off'
                        {...field}
                        onChange={(event) =>
                          field.onChange(event.target.value)
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Alibaba Cloud AccessKey ID.')}
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
                    <FormLabel>{t('Access Key Secret')}</FormLabel>
                    <FormControl>
                      <Input
                        type='password'
                        placeholder={t('Enter new key to update')}
                        autoComplete='new-password'
                        {...field}
                        onChange={(event) =>
                          field.onChange(event.target.value)
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Leave blank to keep the existing key')}
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
                    <FormLabel>{t('Path Prefix')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('images/gemini/')}
                        autoComplete='off'
                        {...field}
                        onChange={(event) =>
                          field.onChange(event.target.value)
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Optional prefix for uploaded image paths. Example: images/gemini/'
                      )}
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
                    <FormLabel>{t('Public Base URL')}</FormLabel>
                    <FormControl>
                      <Input
                        type='url'
                        inputMode='url'
                        placeholder={t(
                          'https://my-bucket.oss-cn-hangzhou.aliyuncs.com'
                        )}
                        autoComplete='off'
                        {...field}
                        onChange={(event) =>
                          field.onChange(event.target.value)
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Public access URL prefix. If not set, the endpoint + bucket will be used.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </>
          )}
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

/*
Copyright (C) 2025 QuantumNous

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

import React, { useEffect, useRef, useState } from 'react';
import { Button, Col, Form, Row, Spin } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  API,
  compareObjects,
  showError,
  showSuccess,
  showWarning,
} from '../../../helpers';

const initialInputs = {
  AliyunOssEnabled: false,
  AliyunOssEndpoint: '',
  AliyunOssBucket: '',
  AliyunOssAccessKeyId: '',
  AliyunOssAccessKeySecret: '',
  AliyunOssPathPrefix: '',
  AliyunOssPublicBaseUrl: '',
};

function normalizeUrl(value) {
  return String(value || '').trim().replace(/\/+$/, '');
}

export default function SettingsAliyunOss(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState(initialInputs);
  const [inputsRow, setInputsRow] = useState(initialInputs);
  const refForm = useRef();

  function handleFieldChange(fieldName, normalize) {
    return (value) => {
      setInputs((current) => ({
        ...current,
        [fieldName]: normalize ? normalize(value) : value,
      }));
    };
  }

  function buildUpdateQueue(updateArray) {
    return updateArray
      .filter((item) => {
        return (
          item.key !== 'AliyunOssAccessKeySecret' ||
          String(inputs.AliyunOssAccessKeySecret || '').trim() !== ''
        );
      })
      .map((item) => {
        let value = inputs[item.key];
        if (
          item.key === 'AliyunOssEndpoint' ||
          item.key === 'AliyunOssPublicBaseUrl'
        ) {
          value = normalizeUrl(value);
        } else if (typeof value === 'string') {
          value = value.trim();
        }
        if (typeof value === 'boolean') {
          value = String(value);
        }
        return API.put('/api/option/', {
          key: item.key,
          value,
        });
      });
  }

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));

    const requestQueue = buildUpdateQueue(updateArray);
    if (!requestQueue.length) return showWarning(t('你似乎并没有修改什么'));

    setLoading(true);
    Promise.all(requestQueue)
      .then((res) => {
        if (res.includes(undefined)) {
          return showError(t('部分保存失败，请重试'));
        }
        showSuccess(t('保存成功'));
        props.refresh();
      })
      .catch(() => {
        showError(t('保存失败，请重试'));
      })
      .finally(() => {
        setLoading(false);
      });
  }

  useEffect(() => {
    const currentInputs = { ...initialInputs };
    for (let key in props.options) {
      if (Object.keys(initialInputs).includes(key)) {
        currentInputs[key] = props.options[key];
      }
    }
    // 后端不会返回 Secret；留空表示保留现有密钥。
    currentInputs.AliyunOssAccessKeySecret = '';

    setInputs(currentInputs);
    setInputsRow(structuredClone(currentInputs));
    refForm.current?.setValues(currentInputs);
  }, [props.options]);

  return (
    <Spin spinning={loading}>
      <Form
        values={inputs}
        getFormApi={(formAPI) => (refForm.current = formAPI)}
        style={{ marginBottom: 15 }}
      >
        <Form.Section
          text={t('阿里云 OSS 设置')}
          extraText={t(
            '开启后，Gemini 渠道返回的 inline 图片会上传到阿里云 OSS，并替换为公开 URL',
          )}
        >
          <Row gutter={16}>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.Switch
                field='AliyunOssEnabled'
                label={t('启用阿里云 OSS')}
                size='default'
                checkedText='｜'
                uncheckedText='〇'
                onChange={handleFieldChange('AliyunOssEnabled')}
              />
            </Col>
          </Row>

          {inputs.AliyunOssEnabled && (
            <>
              <Row gutter={16}>
                <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                  <Form.Input
                    field='AliyunOssEndpoint'
                    label={t('OSS Endpoint')}
                    placeholder='https://oss-cn-hangzhou.aliyuncs.com'
                    extraText={t(
                      '阿里云 OSS Endpoint，例如 https://oss-cn-hangzhou.aliyuncs.com',
                    )}
                    onChange={handleFieldChange('AliyunOssEndpoint')}
                    showClear
                  />
                </Col>
                <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                  <Form.Input
                    field='AliyunOssBucket'
                    label={t('OSS Bucket')}
                    placeholder='my-bucket-name'
                    extraText={t('用于保存图片的 OSS Bucket 名称')}
                    onChange={handleFieldChange('AliyunOssBucket')}
                    showClear
                  />
                </Col>
                <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                  <Form.Input
                    field='AliyunOssAccessKeyId'
                    label={t('AccessKey ID')}
                    placeholder='LTAI...'
                    extraText={t('阿里云 AccessKey ID')}
                    onChange={handleFieldChange('AliyunOssAccessKeyId')}
                    showClear
                  />
                </Col>
              </Row>

              <Row gutter={16}>
                <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                  <Form.Input
                    field='AliyunOssAccessKeySecret'
                    label={t('AccessKey Secret')}
                    placeholder={t('输入新密钥以更新')}
                    extraText={t('留空则保留现有密钥')}
                    mode='password'
                    onChange={handleFieldChange('AliyunOssAccessKeySecret')}
                    showClear
                  />
                </Col>
                <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                  <Form.Input
                    field='AliyunOssPathPrefix'
                    label={t('路径前缀')}
                    placeholder='images/gemini/'
                    extraText={t('可选，上传图片对象路径前缀')}
                    onChange={handleFieldChange('AliyunOssPathPrefix')}
                    showClear
                  />
                </Col>
                <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                  <Form.Input
                    field='AliyunOssPublicBaseUrl'
                    label={t('公开访问地址')}
                    placeholder='https://my-bucket.oss-cn-hangzhou.aliyuncs.com'
                    extraText={t(
                      '可选，公开访问 URL 前缀；不填则使用 Bucket + Endpoint',
                    )}
                    onChange={handleFieldChange('AliyunOssPublicBaseUrl')}
                    showClear
                  />
                </Col>
              </Row>
            </>
          )}

          <Row>
            <Button size='default' onClick={onSubmit}>
              {t('保存阿里云 OSS 设置')}
            </Button>
          </Row>
        </Form.Section>
      </Form>
    </Spin>
  );
}

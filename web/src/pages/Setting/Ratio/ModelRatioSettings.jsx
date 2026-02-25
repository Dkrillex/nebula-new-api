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

import React, { useEffect, useState, useRef } from 'react';
import {
  Button,
  Col,
  Form,
  Popconfirm,
  Row,
  Space,
  Spin,
} from '@douyinfe/semi-ui';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
  verifyJSON,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

export default function ModelRatioSettings(props) {
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    ModelPrice: '',
    ModelRatio: '',
    CacheRatio: '',
    CompletionRatio: '',
    ImageRatio: '',
    AudioRatio: '',
    AudioCompletionRatio: '',
    ImageCompletionRatio: '',
    OriginImageCompletionRatio: '',
    ExposeRatioEnabled: false,
    OriginModelPrice: '',
    OriginModelRatio: '',
    OriginCompletionRatio: '',
    VideoModelPricePerSecond: '',
    OriginVideoModelPricePerSecond: '',
    ImageTokenPricing: '',
    OriginImageTokenPricing: '',
    ImageModelPricePerImage: '',
    OriginImageModelPricePerImage: '',
  });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);
  const { t } = useTranslation();

  async function onSubmit() {
    try {
      await refForm.current
        .validate()
        .then(() => {
          // 标准化：去除首尾空白；对JSON类字段将空串提交为"{}"
          const normalize = (key, v) => {
            if (typeof v === 'boolean') return String(v);
            const str = (v || '').toString().trim();
            const jsonKeys = [
              'ModelPrice',
              'OriginModelPrice',
              'ModelRatio',
              'CompletionRatio',
              'OriginModelRatio',
              'OriginCompletionRatio',
              'CacheRatio',
              'ImageRatio',
              'AudioRatio',
              'AudioCompletionRatio',
              'ImageCompletionRatio',
              'OriginImageCompletionRatio',
              'VideoModelPricePerSecond',
              'OriginVideoModelPricePerSecond',
              'ImageTokenPricing',
              'OriginImageTokenPricing',
              'ImageModelPricePerImage',
              'OriginImageModelPricePerImage',
            ];
            if (jsonKeys.includes(key) && str === '') return '{}';
            return str;
          };

          // 逐字段比较（标准化后）
          const changedKeys = Object.keys(inputs).filter((k) =>
            normalize(k, inputs[k]) !== normalize(k, inputsRow[k]),
          );
          if (!changedKeys.length)
            return showWarning(t('你似乎并没有修改什么'));

          const requestQueue = changedKeys.map((key) => {
            const value = normalize(key, inputs[key]);
            return API.put('/api/option/', { key, value });
          });

          setLoading(true);
          Promise.all(requestQueue)
            .then((res) => {
              if (res.includes(undefined)) {
                return showError(
                  requestQueue.length > 1
                    ? t('部分保存失败，请重试')
                    : t('保存失败'),
                );
              }

              for (let i = 0; i < res.length; i++) {
                if (!res[i].data.success) {
                  return showError(res[i].data.message);
                }
              }

              showSuccess(t('保存成功'));
              props.refresh();
            })
            .catch((error) => {
              console.error('Unexpected error:', error);
              showError(t('保存失败，请重试'));
            })
            .finally(() => {
              setLoading(false);
            });
        })
        .catch(() => {
          showError(t('请检查输入'));
        });
    } catch (error) {
      showError(t('请检查输入'));
      console.error(error);
    }
  }

  async function resetModelRatio() {
    try {
      let res = await API.post(`/api/option/rest_model_ratio`);
      if (res.data.success) {
        showSuccess(res.data.message);
        props.refresh();
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(error);
    }
  }

  useEffect(() => {
    const currentInputs = {};
    for (let key in props.options) {
      if (Object.keys(inputs).includes(key)) {
        currentInputs[key] = props.options[key];
      }
    }
    // 确保新添加的字段有默认值，即使props.options中没有
    if (!currentInputs.hasOwnProperty('OriginModelPrice')) {
      currentInputs.OriginModelPrice = '';
    }
    if (!currentInputs.hasOwnProperty('OriginModelRatio')) {
      currentInputs.OriginModelRatio = '';
    }
    if (!currentInputs.hasOwnProperty('OriginCompletionRatio')) {
      currentInputs.OriginCompletionRatio = '';
    }
    if (!currentInputs.hasOwnProperty('VideoModelPricePerSecond')) {
      currentInputs.VideoModelPricePerSecond = '';
    }
    if (!currentInputs.hasOwnProperty('OriginVideoModelPricePerSecond')) {
      currentInputs.OriginVideoModelPricePerSecond = '';
    }
    if (!currentInputs.hasOwnProperty('ImageTokenPricing')) {
      currentInputs.ImageTokenPricing = '';
    }
    if (!currentInputs.hasOwnProperty('OriginImageTokenPricing')) {
      currentInputs.OriginImageTokenPricing = '';
    }
    if (!currentInputs.hasOwnProperty('ImageModelPricePerImage')) {
      currentInputs.ImageModelPricePerImage = '';
    }
    if (!currentInputs.hasOwnProperty('OriginImageModelPricePerImage')) {
      currentInputs.OriginImageModelPricePerImage = '';
    }
    if (!currentInputs.hasOwnProperty('ImageCompletionRatio')) {
      currentInputs.ImageCompletionRatio = '';
    }
    if (!currentInputs.hasOwnProperty('OriginImageCompletionRatio')) {
      currentInputs.OriginImageCompletionRatio = '';
    }
    setInputs(currentInputs);
    setInputsRow(structuredClone(currentInputs));
    refForm.current.setValues(currentInputs);
  }, [props.options]);

  return (
    <Spin spinning={loading}>
      <Form
        values={inputs}
        getFormApi={(formAPI) => (refForm.current = formAPI)}
        style={{ marginBottom: 15 }}
      >
        <Row gutter={16} style={{ display: 'flex', alignItems: 'stretch' }}>
          <Col xs={24} sm={12} style={{ display: 'flex', flexDirection: 'column' }}>
            <Form.TextArea
              label={t('模型固定价格')}
              extraText={t('一次调用消耗多少刀，优先级大于模型倍率')}
              placeholder={t(
                '为一个 JSON 文本，键为模型名称，值为一次调用消耗多少刀，比如 "gpt-4-gizmo-*": 0.1，一次消耗0.1刀',
              )}
              field={'ModelPrice'}
              autosize={{ minRows: 6, maxRows: 12 }}
              style={{ flex: 1 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) => setInputs({ ...inputs, ModelPrice: value })}
            />
          </Col>
          {/*
          <Col xs={24} sm={12} style={{ display: 'flex', flexDirection: 'column' }}>
            <Form.TextArea
              label={t('原始模型固定价格')}
              extraText={t('原始模型固定价格')}
              placeholder={t(
                '为一个 JSON 文本，键为模型名称，值为一次调用消耗多少刀，比如 "gpt-4-gizmo-*": 0.1，一次消耗0.1刀',
              )}
              field={'OriginModelPrice'}
              autosize={{ minRows: 6, maxRows: 12 }}
              style={{ flex: 1 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) =>
                setInputs({ ...inputs, OriginModelPrice: value })
              }
            />
          </Col>
          */}
        </Row>
        <Row gutter={16} style={{ display: 'flex', alignItems: 'stretch' }}>
          <Col xs={24} sm={12} style={{ display: 'flex', flexDirection: 'column' }}>
            <Form.TextArea
              label={t('模型倍率')}
              placeholder={t('为一个 JSON 文本，键为模型名称，值为倍率')}
              field={'ModelRatio'}
              autosize={{ minRows: 6, maxRows: 12 }}
              style={{ flex: 1 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) => setInputs({ ...inputs, ModelRatio: value })}
            />
          </Col>
          {/*
          <Col xs={24} sm={12} style={{ display: 'flex', flexDirection: 'column' }}>
            <Form.TextArea
              label={t('原始模型倍率')}
              placeholder={t('为一个 JSON 文本，键为模型名称，值为倍率')}
              field={'OriginModelRatio'}
              autosize={{ minRows: 6, maxRows: 12 }}
              style={{ flex: 1 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) =>
                setInputs({ ...inputs, OriginModelRatio: value })
              }
            />
          </Col>
          */}
        </Row>
        <Row gutter={16} style={{ display: 'flex', alignItems: 'stretch' }}>
          <Col xs={24} sm={12} style={{ display: 'flex', flexDirection: 'column' }}>
            <Form.TextArea
              label={t('模型补全倍率（仅对自定义模型有效）')}
              extraText={t('仅对自定义模型有效')}
              placeholder={t('为一个 JSON 文本，键为模型名称，值为倍率')}
              field={'CompletionRatio'}
              autosize={{ minRows: 6, maxRows: 12 }}
              style={{ flex: 1 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) =>
                setInputs({ ...inputs, CompletionRatio: value })
              }
            />
          </Col>
          {/*
          <Col xs={24} sm={12} style={{ display: 'flex', flexDirection: 'column' }}>
            <Form.TextArea
              label={t('原始模型补全倍率')}
              placeholder={t('为一个 JSON 文本，键为模型名称，值为倍率')}
              field={'OriginCompletionRatio'}
              autosize={{ minRows: 6, maxRows: 12 }}
              style={{ flex: 1 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) =>
                setInputs({ ...inputs, OriginCompletionRatio: value })
              }
            />
          </Col>
          */}
        </Row>
        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('提示缓存倍率')}
              placeholder={t('为一个 JSON 文本，键为模型名称，值为倍率')}
              field={'CacheRatio'}
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) => setInputs({ ...inputs, CacheRatio: value })}
            />
          </Col>
        </Row>
        <Row gutter={16} style={{ display: 'flex', alignItems: 'stretch' }}>
          <Col xs={24} sm={12} style={{ display: 'flex', flexDirection: 'column' }}>
            <Form.TextArea
              label={t('视频模型每秒价格')}
              extraText={t('视频模型按秒计费的价格，单位：美元/秒')}
              placeholder={t(
                '为一个 JSON 文本，键为模型名称，值为每秒价格，比如 "sora-2": 0.1，表示每秒0.1美元',
              )}
              field={'VideoModelPricePerSecond'}
              autosize={{ minRows: 6, maxRows: 12 }}
              style={{ flex: 1 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) =>
                setInputs({ ...inputs, VideoModelPricePerSecond: value })
              }
            />
          </Col>
          {/*
          <Col xs={24} sm={12} style={{ display: 'flex', flexDirection: 'column' }}>
            <Form.TextArea
              label={t('原始视频模型每秒价格')}
              extraText={t('原始视频模型按秒计费的价格，单位：美元/秒')}
              placeholder={t(
                '为一个 JSON 文本，键为模型名称，值为每秒价格，用于对比展示',
              )}
              field={'OriginVideoModelPricePerSecond'}
              autosize={{ minRows: 6, maxRows: 12 }}
              style={{ flex: 1 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) =>
                setInputs({ ...inputs, OriginVideoModelPricePerSecond: value })
              }
            />
          </Col>
          */}
        </Row>
        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('图片输入倍率（仅部分模型支持该计费）')}
              extraText={t(
                '图片输入相关的倍率设置，键为模型名称，值为倍率，仅部分模型支持该计费',
              )}
              placeholder={t(
                '为一个 JSON 文本，键为模型名称，值为倍率，例如：{"gpt-image-1": 2}',
              )}
              field={'ImageRatio'}
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) => setInputs({ ...inputs, ImageRatio: value })}
            />
          </Col>
        </Row>
        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('音频倍率（仅部分模型支持该计费）')}
              extraText={t('音频输入相关的倍率设置，键为模型名称，值为倍率')}
              placeholder={t(
                '为一个 JSON 文本，键为模型名称，值为倍率，例如：{"gpt-4o-audio-preview": 16}',
              )}
              field={'AudioRatio'}
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) => setInputs({ ...inputs, AudioRatio: value })}
            />
          </Col>
        </Row>
        <Row gutter={16}>
          <Col xs={24} sm={16}>
            <Form.TextArea
              label={t('音频补全倍率（仅部分模型支持该计费）')}
              extraText={t(
                '音频输出补全相关的倍率设置，键为模型名称，值为倍率',
              )}
              placeholder={t(
                '为一个 JSON 文本，键为模型名称，值为倍率，例如：{"gpt-4o-realtime": 2}',
              )}
              field={'AudioCompletionRatio'}
              autosize={{ minRows: 6, maxRows: 12 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) =>
                setInputs({ ...inputs, AudioCompletionRatio: value })
              }
            />
          </Col>
        </Row>
        <Row gutter={16} style={{ display: 'flex', alignItems: 'stretch' }}>
          <Col xs={24} sm={12} style={{ display: 'flex', flexDirection: 'column' }}>
            <Form.TextArea
              label={t('图片补全倍率')}
              extraText={t(
                '图片输出相关的倍率设置，用于同时返回文本和图片的模型（如 gemini-3-pro-image-preview）',
              )}
              placeholder={t(
                '为一个 JSON 文本，键为模型名称，值为倍率，例如：{"gemini-3-pro-image-preview": 45}',
              )}
              field={'ImageCompletionRatio'}
              autosize={{ minRows: 6, maxRows: 12 }}
              style={{ flex: 1 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) =>
                setInputs({ ...inputs, ImageCompletionRatio: value })
              }
            />
          </Col>
          {/*
          <Col xs={24} sm={12} style={{ display: 'flex', flexDirection: 'column' }}>
            <Form.TextArea
              label={t('原始图片补全倍率')}
              extraText={t('原始图片补全倍率，用于前端展示对比')}
              placeholder={t(
                '为一个 JSON 文本，键为模型名称，值为倍率，例如：{"gemini-3-pro-image-preview": 60}',
              )}
              field={'OriginImageCompletionRatio'}
              autosize={{ minRows: 6, maxRows: 12 }}
              style={{ flex: 1 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) =>
                setInputs({ ...inputs, OriginImageCompletionRatio: value })
              }
            />
          </Col>
          */}
        </Row>
        <Row gutter={16} style={{ display: 'flex', alignItems: 'stretch' }}>
          <Col xs={24} sm={12} style={{ display: 'flex', flexDirection: 'column' }}>
            <Form.TextArea
              label={t('图像Token表定价')}
              extraText={t(
                '用于 gpt-image-1 等特殊图像模型，包含三种价格（输入文本/输入图像/输出图像）和固定Token表',
              )}
              placeholder={t(
                '为一个 JSON 文本，格式：{"gpt-image-1": {"input_text_price": 5.0, "input_image_price": 10.0, "output_image_price": 40.0, "token_table": {"medium": {"1024x1024": 1056}}}}',
              )}
              field={'ImageTokenPricing'}
              autosize={{ minRows: 6, maxRows: 12 }}
              style={{ flex: 1 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) =>
                setInputs({ ...inputs, ImageTokenPricing: value })
              }
            />
          </Col>
          {/*
          <Col xs={24} sm={12} style={{ display: 'flex', flexDirection: 'column' }}>
            <Form.TextArea
              label={t('原始图像Token表定价')}
              extraText={t(
                '微软官方原价，用于在模型广场中展示原价和优惠价对比，吸引用户',
              )}
              placeholder={t(
                '为一个 JSON 文本，格式与 ImageTokenPricing 相同',
              )}
              field={'OriginImageTokenPricing'}
              autosize={{ minRows: 6, maxRows: 12 }}
              style={{ flex: 1 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) =>
                setInputs({ ...inputs, OriginImageTokenPricing: value })
              }
            />
          </Col>
          */}
        </Row>
        <Row gutter={16} style={{ display: 'flex', alignItems: 'stretch' }}>
          <Col xs={24} sm={12} style={{ display: 'flex', flexDirection: 'column' }}>
            <Form.TextArea
              label={t('图片模型按张计费价格（系统价格）')}
              extraText={t(
                '用于实际扣费，单位：美元/张。例如 {"qwen-image-plus": 0.0247}',
              )}
              placeholder={t(
                '为一个 JSON 文本，键为模型名称，值为每张图片的价格（美元）',
              )}
              field={'ImageModelPricePerImage'}
              autosize={{ minRows: 3, maxRows: 8 }}
              style={{ flex: 1 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) =>
                setInputs({ ...inputs, ImageModelPricePerImage: value })
              }
            />
          </Col>
          {/*
          <Col xs={24} sm={12} style={{ display: 'flex', flexDirection: 'column' }}>
            <Form.TextArea
              label={t('图片模型按张计费原始价格')}
              extraText={t(
                '厂商官方原价，用于在模型广场中展示价格对比，吸引用户，单位：美元/张',
              )}
              placeholder={t(
                '为一个 JSON 文本，格式与 ImageModelPricePerImage 相同',
              )}
              field={'OriginImageModelPricePerImage'}
              autosize={{ minRows: 3, maxRows: 8 }}
              style={{ flex: 1 }}
              trigger='blur'
              stopValidateWithError
              rules={[
                {
                  validator: (rule, value) => verifyJSON(value),
                  message: '不是合法的 JSON 字符串',
                },
              ]}
              onChange={(value) =>
                setInputs({ ...inputs, OriginImageModelPricePerImage: value })
              }
            />
          </Col>
          */}
        </Row>
        <Row gutter={16}>
          <Col span={16}>
            <Form.Switch
              label={t('暴露倍率接口')}
              field={'ExposeRatioEnabled'}
              onChange={(value) =>
                setInputs({ ...inputs, ExposeRatioEnabled: value })
              }
            />
          </Col>
        </Row>
      </Form>
      <Space>
        <Button onClick={onSubmit}>{t('保存模型倍率设置')}</Button>
        <Popconfirm
          title={t('确定重置模型倍率吗？')}
          content={t('此修改将不可逆')}
          okType={'danger'}
          position={'top'}
          onConfirm={resetModelRatio}
        >
          <Button type={'danger'}>{t('重置模型倍率')}</Button>
        </Popconfirm>
      </Space>
    </Spin>
  );
}

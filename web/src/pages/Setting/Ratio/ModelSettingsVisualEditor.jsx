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
  Table,
  Button,
  Input,
  Modal,
  Form,
  Space,
  RadioGroup,
  Radio,
  Checkbox,
  Tag,
} from '@douyinfe/semi-ui';
import {
  IconDelete,
  IconPlus,
  IconSearch,
  IconSave,
  IconEdit,
} from '@douyinfe/semi-icons';
import { API, showError, showSuccess, getQuotaPerUnit } from '../../../helpers';
import { useTranslation } from 'react-i18next';

export default function ModelSettingsVisualEditor(props) {
  const { t } = useTranslation();
  const [models, setModels] = useState([]);
  const [visible, setVisible] = useState(false);
  const [isEditMode, setIsEditMode] = useState(false);
  const [currentModel, setCurrentModel] = useState(null);
  const [searchText, setSearchText] = useState('');
  const [currentPage, setCurrentPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [pricingMode, setPricingMode] = useState('per-token'); // 'per-token' or 'per-request'
  const [pricingSubMode, setPricingSubMode] = useState('ratio'); // 'ratio' or 'token-price'
  const [conflictOnly, setConflictOnly] = useState(false);
  const formRef = useRef(null);
  const pageSize = 10;
  const quotaPerUnit = getQuotaPerUnit();

  useEffect(() => {
    try {
      const modelPrice = JSON.parse(props.options.ModelPrice || '{}');
      const modelRatio = JSON.parse(props.options.ModelRatio || '{}');
      const completionRatio = JSON.parse(props.options.CompletionRatio || '{}');
      const originModelPrice = JSON.parse(
        props.options.OriginModelPrice || '{}',
      );
      const originModelRatio = JSON.parse(
        props.options.OriginModelRatio || '{}',
      );
      const originCompletionRatio = JSON.parse(
        props.options.OriginCompletionRatio || '{}',
      );
      const videoModelPricePerSecond = JSON.parse(
        props.options.VideoModelPricePerSecond || '{}',
      );
      const imageTokenPricing = JSON.parse(
        props.options.ImageTokenPricing || '{}',
      );
      const originImageTokenPricing = JSON.parse(
        props.options.OriginImageTokenPricing || '{}',
      );

      // 合并所有模型名称
      const modelNames = new Set([
        ...Object.keys(modelPrice),
        ...Object.keys(modelRatio),
        ...Object.keys(completionRatio),
        ...Object.keys(originModelPrice),
        ...Object.keys(originModelRatio),
        ...Object.keys(originCompletionRatio),
        ...Object.keys(videoModelPricePerSecond),
        ...Object.keys(imageTokenPricing),
        ...Object.keys(originImageTokenPricing),
      ]);

      const modelData = Array.from(modelNames).map((name) => {
        const price = modelPrice[name] === undefined ? '' : modelPrice[name];
        const ratio = modelRatio[name] === undefined ? '' : modelRatio[name];
        const comp =
          completionRatio[name] === undefined ? '' : completionRatio[name];
        const originPrice =
          originModelPrice[name] === undefined ? '' : originModelPrice[name];
        const originRatio =
          originModelRatio[name] === undefined ? '' : originModelRatio[name];
        const originComp =
          originCompletionRatio[name] === undefined
            ? ''
            : originCompletionRatio[name];
        const videoPrice =
          videoModelPricePerSecond[name] === undefined
            ? ''
            : videoModelPricePerSecond[name];
        const imageTokenPricingData = imageTokenPricing[name] || null;
        const originImageTokenPricingData =
          originImageTokenPricing[name] || null;

        // 计算原始输入价格（从原始模型倍率转换）
        const originTokenPrice =
          originRatio !== '' ? (parseFloat(originRatio) * 2).toString() : '';

        // 计算原始输出价格（从原始模型补全倍率转换）
        const originCompletionTokenPrice =
          originComp !== '' && originTokenPrice !== ''
            ? (parseFloat(originComp) * parseFloat(originTokenPrice)).toString()
            : '';

        // 检测冲突：四种定价方式互斥（price、videoPrice、ratio、imageTokenPricing）
        const pricingMethodsCount = [
          price !== '',
          videoPrice !== '',
          ratio !== '' || comp !== '',
          imageTokenPricingData !== null,
        ].filter(Boolean).length;

        return {
          name,
          price,
          ratio,
          completionRatio: comp,
          originPrice,
          originRatio,
          originCompletionRatio: originComp,
          originTokenPrice,
          originCompletionTokenPrice,
          videoPrice,
          imageTokenPricingData,
          originImageTokenPricingData,
          hasConflict: pricingMethodsCount > 1,
        };
      });

      setModels(modelData);
    } catch (error) {
      console.error('JSON解析错误:', error);
    }
  }, [props.options]);

  // 首先声明分页相关的工具函数
  const getPagedData = (data, currentPage, pageSize) => {
    const start = (currentPage - 1) * pageSize;
    const end = start + pageSize;
    return data.slice(start, end);
  };

  // 在 return 语句之前，先处理过滤和分页逻辑
  const filteredModels = models.filter((model) => {
    const keywordMatch = searchText ? model.name.includes(searchText) : true;
    const conflictMatch = conflictOnly ? model.hasConflict : true;
    return keywordMatch && conflictMatch;
  });

  // 然后基于过滤后的数据计算分页数据
  const pagedData = getPagedData(filteredModels, currentPage, pageSize);

  const SubmitData = async () => {
    setLoading(true);
    const output = {
      ModelPrice: {},
      ModelRatio: {},
      CompletionRatio: {},
      OriginModelPrice: {},
      OriginModelRatio: {},
      OriginCompletionRatio: {},
      VideoModelPricePerSecond: {},
      ImageTokenPricing: {},
      OriginImageTokenPricing: {},
    };
    let currentConvertModelName = '';

    try {
      // 数据转换
      models.forEach((model) => {
        currentConvertModelName = model.name;
        // 四种定价方式互斥：imageTokenPricing > videoPrice > price > ratio
        if (model.imageTokenPricingData) {
          // 图像Token表定价
          output.ImageTokenPricing[model.name] = model.imageTokenPricingData;
          if (model.originImageTokenPricingData) {
            output.OriginImageTokenPricing[model.name] =
              model.originImageTokenPricingData;
          }
        } else if (model.videoPrice !== '') {
          // 如果视频价格不为空，则转换为浮点数，忽略其他价格和倍率参数
          output.VideoModelPricePerSecond[model.name] = parseFloat(
            model.videoPrice,
          );
        } else if (model.price !== '') {
          // 如果固定价格不为空，则转换为浮点数，忽略倍率参数
          output.ModelPrice[model.name] = parseFloat(model.price);
        } else {
          if (model.ratio !== '')
            output.ModelRatio[model.name] = parseFloat(model.ratio);
          if (model.completionRatio !== '')
            output.CompletionRatio[model.name] = parseFloat(
              model.completionRatio,
            );
        }

        // 处理原始模型配置（仅在非图像Token表计费时）
        if (!model.imageTokenPricingData) {
          if (model.originPrice !== '')
            output.OriginModelPrice[model.name] = parseFloat(model.originPrice);
          if (model.originRatio !== '')
            output.OriginModelRatio[model.name] = parseFloat(model.originRatio);
          if (model.originCompletionRatio !== '')
            output.OriginCompletionRatio[model.name] = parseFloat(
              model.originCompletionRatio,
            );

          // 如果有原始输入价格，转换为原始模型倍率存储
          if (model.originTokenPrice !== '') {
            const originTokenPrice = parseFloat(model.originTokenPrice);
            const originRatio = originTokenPrice / 2; // 使用相同的转换逻辑
            output.OriginModelRatio[model.name] = originRatio;
          }
        }
      });

      // 准备API请求数组
      const finalOutput = {
        ModelPrice: JSON.stringify(output.ModelPrice, null, 2),
        ModelRatio: JSON.stringify(output.ModelRatio, null, 2),
        CompletionRatio: JSON.stringify(output.CompletionRatio, null, 2),
        OriginModelPrice: JSON.stringify(output.OriginModelPrice, null, 2),
        OriginModelRatio: JSON.stringify(output.OriginModelRatio, null, 2),
        OriginCompletionRatio: JSON.stringify(
          output.OriginCompletionRatio,
          null,
          2,
        ),
        VideoModelPricePerSecond: JSON.stringify(
          output.VideoModelPricePerSecond,
          null,
          2,
        ),
        ImageTokenPricing: JSON.stringify(output.ImageTokenPricing, null, 2),
        OriginImageTokenPricing: JSON.stringify(
          output.OriginImageTokenPricing,
          null,
          2,
        ),
      };

      const requestQueue = Object.entries(finalOutput).map(([key, value]) => {
        return API.put('/api/option/', {
          key,
          value,
        });
      });

      // 批量处理请求
      const results = await Promise.all(requestQueue);

      // 验证结果
      if (requestQueue.length === 1) {
        if (results.includes(undefined)) return;
      } else if (requestQueue.length > 1) {
        if (results.includes(undefined)) {
          return showError('部分保存失败，请重试');
        }
      }

      // 检查每个请求的结果
      for (const res of results) {
        if (!res.data.success) {
          return showError(res.data.message);
        }
      }

      showSuccess('保存成功');
      props.refresh();
    } catch (error) {
      console.error('保存失败:', error);
      showError('保存失败，请重试');
    } finally {
      setLoading(false);
    }
  };

  const columns = [
    {
      title: t('模型名称'),
      dataIndex: 'name',
      key: 'name',
      render: (text, record) => (
        <span>
          {text}
          {record.hasConflict && (
            <Tag color='red' shape='circle' className='ml-2'>
              {t('矛盾')}
            </Tag>
          )}
        </span>
      ),
    },
    {
      title: t('模型固定价格'),
      dataIndex: 'price',
      key: 'price',
      render: (text, record) => (
        <Input
          value={text}
          placeholder={t('按量计费')}
          disabled={record.videoPrice !== ''}
          onChange={(value) => updateModel(record.name, 'price', value)}
        />
      ),
    },
    {
      title: t('视频每秒价格'),
      dataIndex: 'videoPrice',
      key: 'videoPrice',
      render: (text, record) => (
        <Input
          value={text}
          placeholder={t('视频按秒计费')}
          disabled={record.price !== ''}
          onChange={(value) => updateModel(record.name, 'videoPrice', value)}
        />
      ),
    },
    {
      title: t('模型倍率'),
      dataIndex: 'ratio',
      key: 'ratio',
      render: (text, record) => (
        <Input
          value={text}
          placeholder={
            record.price !== '' || record.videoPrice !== ''
              ? t('模型倍率')
              : t('默认补全倍率')
          }
          disabled={record.price !== '' || record.videoPrice !== ''}
          onChange={(value) => updateModel(record.name, 'ratio', value)}
        />
      ),
    },
    {
      title: t('补全倍率'),
      dataIndex: 'completionRatio',
      key: 'completionRatio',
      render: (text, record) => (
        <Input
          value={text}
          placeholder={
            record.price !== '' || record.videoPrice !== ''
              ? t('补全倍率')
              : t('默认补全倍率')
          }
          disabled={record.price !== '' || record.videoPrice !== ''}
          onChange={(value) =>
            updateModel(record.name, 'completionRatio', value)
          }
        />
      ),
    },
    // {
    //   title: t('原始模型固定价格'),
    //   dataIndex: 'originPrice',
    //   key: 'originPrice',
    //   render: (text, record) => (
    //     <Input
    //       value={text}
    //       placeholder={t('原始模型固定价格')}
    //       onChange={(value) => updateModel(record.name, 'originPrice', value)}
    //     />
    //   ),
    // },
    // {
    //   title: t('原始模型倍率'),
    //   dataIndex: 'originRatio',
    //   key: 'originRatio',
    //   render: (text, record) => (
    //     <Input
    //       value={text}
    //       placeholder={t('原始模型倍率')}
    //       onChange={(value) => updateModel(record.name, 'originRatio', value)}
    //     />
    //   ),
    // },
    // {
    //   title: t('原始模型补全倍率'),
    //   dataIndex: 'originCompletionRatio',
    //   key: 'originCompletionRatio',
    //   render: (text, record) => (
    //     <Input
    //       value={text}
    //       placeholder={t('原始模型补全倍率')}
    //       onChange={(value) => updateModel(record.name, 'originCompletionRatio', value)}
    //     />
    //   ),
    // },
    {
      title: t('操作'),
      key: 'action',
      render: (_, record) => (
        <Space>
          <Button
            type='primary'
            icon={<IconEdit />}
            onClick={() => editModel(record)}
          ></Button>
          <Button
            icon={<IconDelete />}
            type='danger'
            onClick={() => deleteModel(record.name)}
          />
        </Space>
      ),
    },
  ];

  const updateModel = (name, field, value) => {
    if (value !== '' && isNaN(value)) {
      showError('请输入数字');
      return;
    }
    setModels((prev) =>
      prev.map((model) => {
        if (model.name !== name) return model;
        const updated = { ...model, [field]: value };
        updated.hasConflict =
          (updated.price !== '' || updated.videoPrice !== '') &&
          (updated.ratio !== '' || updated.completionRatio !== '');
        return updated;
      }),
    );
  };

  const deleteModel = (name) => {
    setModels((prev) => prev.filter((model) => model.name !== name));
  };

  const calculateRatioFromTokenPrice = (tokenPrice) => {
    return tokenPrice / 2;
  };

  const calculateCompletionRatioFromPrices = (
    modelTokenPrice,
    completionTokenPrice,
  ) => {
    if (!modelTokenPrice || modelTokenPrice === '0') {
      showError('模型价格不能为0');
      return '';
    }
    return completionTokenPrice / modelTokenPrice;
  };

  const handleTokenPriceChange = (value) => {
    // Use a temporary variable to hold the new state
    let newState = {
      ...(currentModel || {}),
      tokenPrice: value,
      ratio: 0,
    };

    if (!isNaN(value) && value !== '') {
      const tokenPrice = parseFloat(value);
      const ratio = calculateRatioFromTokenPrice(tokenPrice);
      newState.ratio = ratio;
    }

    // Set the state with the complete updated object
    setCurrentModel(newState);
  };

  const handleCompletionTokenPriceChange = (value) => {
    // Use a temporary variable to hold the new state
    let newState = {
      ...(currentModel || {}),
      completionTokenPrice: value,
      completionRatio: 0,
    };

    if (!isNaN(value) && value !== '' && currentModel?.tokenPrice) {
      const completionTokenPrice = parseFloat(value);
      const modelTokenPrice = parseFloat(currentModel.tokenPrice);

      if (modelTokenPrice > 0) {
        const completionRatio = calculateCompletionRatioFromPrices(
          modelTokenPrice,
          completionTokenPrice,
        );
        newState.completionRatio = completionRatio;
      }
    }

    // Set the state with the complete updated object
    setCurrentModel(newState);
  };

  const handleOriginTokenPriceChange = (value) => {
    // Use a temporary variable to hold the new state
    let newState = {
      ...(currentModel || {}),
      originTokenPrice: value,
      originRatio: 0,
    };

    if (!isNaN(value) && value !== '') {
      const originTokenPrice = parseFloat(value);
      const originRatio = calculateRatioFromTokenPrice(originTokenPrice);
      newState.originRatio = originRatio;
    }

    // Set the state with the complete updated object
    setCurrentModel(newState);
  };

  const handleOriginCompletionTokenPriceChange = (value) => {
    // Use a temporary variable to hold the new state
    let newState = {
      ...(currentModel || {}),
      originCompletionTokenPrice: value,
      originCompletionRatio: 0,
    };

    if (!isNaN(value) && value !== '' && currentModel?.originTokenPrice) {
      const originCompletionTokenPrice = parseFloat(value);
      const originTokenPrice = parseFloat(currentModel.originTokenPrice);

      if (originTokenPrice > 0) {
        const originCompletionRatio =
          originCompletionTokenPrice / originTokenPrice;
        newState.originCompletionRatio = originCompletionRatio;
      }
    }

    // Set the state with the complete updated object
    setCurrentModel(newState);
  };

  const addOrUpdateModel = (values) => {
    // Check if we're editing an existing model or adding a new one
    const existingModelIndex = models.findIndex(
      (model) => model.name === values.name,
    );

    if (existingModelIndex >= 0) {
      // Update existing model
      setModels((prev) =>
        prev.map((model, index) => {
          if (index !== existingModelIndex) return model;
          const updated = {
            name: values.name,
            price: values.price || '',
            videoPrice: values.videoPrice || '',
            ratio: values.ratio || '',
            completionRatio: values.completionRatio || '',
            originPrice: values.originPrice || '',
            originRatio: values.originRatio || '',
            originCompletionRatio: values.originCompletionRatio || '',
            originTokenPrice: values.originTokenPrice || '',
            originCompletionTokenPrice: values.originCompletionTokenPrice || '',
          };
          updated.hasConflict =
            (updated.price !== '' || updated.videoPrice !== '') &&
            (updated.ratio !== '' || updated.completionRatio !== '');
          return updated;
        }),
      );
      setVisible(false);
      showSuccess(t('更新成功'));
    } else {
      // Add new model
      // Check if model name already exists
      if (models.some((model) => model.name === values.name)) {
        showError(t('模型名称已存在'));
        return;
      }

      setModels((prev) => {
        const newModel = {
          name: values.name,
          price: values.price || '',
          videoPrice: values.videoPrice || '',
          ratio: values.ratio || '',
          completionRatio: values.completionRatio || '',
          originPrice: values.originPrice || '',
          originRatio: values.originRatio || '',
          originCompletionRatio: values.originCompletionRatio || '',
          originTokenPrice: values.originTokenPrice || '',
          originCompletionTokenPrice: values.originCompletionTokenPrice || '',
        };
        newModel.hasConflict =
          (newModel.price !== '' || newModel.videoPrice !== '') &&
          (newModel.ratio !== '' || newModel.completionRatio !== '');
        return [newModel, ...prev];
      });
      setVisible(false);
      showSuccess(t('添加成功'));
    }
  };

  const calculateTokenPriceFromRatio = (ratio) => {
    return ratio * 2;
  };

  const resetModalState = () => {
    setCurrentModel(null);
    setPricingMode('per-token');
    setPricingSubMode('ratio');
    setIsEditMode(false);
  };

  const editModel = (record) => {
    setIsEditMode(true);
    // Determine which pricing mode to use based on the model's current configuration
    let initialPricingMode = 'per-token';
    let initialPricingSubMode = 'ratio';

    if (record.imageTokenPricingData) {
      initialPricingMode = 'per-image-token';
    } else if (record.videoPrice !== '') {
      initialPricingMode = 'per-second';
    } else if (record.price !== '') {
      initialPricingMode = 'per-request';
    } else {
      initialPricingMode = 'per-token';
      // We default to ratio mode, but could set to token-price if needed
    }

    // Set the pricing modes for the form
    setPricingMode(initialPricingMode);
    setPricingSubMode(initialPricingSubMode);

    // Create a copy of the model data to avoid modifying the original
    const modelCopy = { ...record };

    // If the model has ratio data and we want to populate token price fields
    if (record.ratio) {
      modelCopy.tokenPrice = calculateTokenPriceFromRatio(
        parseFloat(record.ratio),
      ).toString();

      if (record.completionRatio) {
        modelCopy.completionTokenPrice = (
          parseFloat(modelCopy.tokenPrice) * parseFloat(record.completionRatio)
        ).toString();
      }
    }

    // Set the current model
    setCurrentModel(modelCopy);

    // Open the modal
    setVisible(true);

    // Use setTimeout to ensure the form is rendered before setting values
    setTimeout(() => {
      if (formRef.current) {
        // Update the form fields based on pricing mode
        const formValues = {
          name: modelCopy.name,
        };

        if (initialPricingMode === 'per-image-token') {
          // 图像Token表定价模式
          if (modelCopy.imageTokenPricingData) {
            formValues.inputTextPrice =
              modelCopy.imageTokenPricingData.input_text_price || '';
            formValues.inputImagePrice =
              modelCopy.imageTokenPricingData.input_image_price || '';
            formValues.outputImagePrice =
              modelCopy.imageTokenPricingData.output_image_price || '';
          }
          if (modelCopy.originImageTokenPricingData) {
            formValues.originInputTextPrice =
              modelCopy.originImageTokenPricingData.input_text_price || '';
            formValues.originInputImagePrice =
              modelCopy.originImageTokenPricingData.input_image_price || '';
            formValues.originOutputImagePrice =
              modelCopy.originImageTokenPricingData.output_image_price || '';
          }
        } else if (initialPricingMode === 'per-second') {
          formValues.videoPriceInput = modelCopy.videoPrice;
        } else if (initialPricingMode === 'per-request') {
          formValues.priceInput = modelCopy.price;
        } else if (initialPricingMode === 'per-token') {
          formValues.ratioInput = modelCopy.ratio;
          formValues.completionRatioInput = modelCopy.completionRatio;
          formValues.modelTokenPrice = modelCopy.tokenPrice;
          formValues.completionTokenPrice = modelCopy.completionTokenPrice;
        }

        // 设置原始模型配置的初始值（仅非图像Token表计费时）
        if (initialPricingMode !== 'per-image-token') {
          formValues.originPrice = modelCopy.originPrice || '';
          formValues.originRatio = modelCopy.originRatio || '';
          formValues.originCompletionRatio =
            modelCopy.originCompletionRatio || '';
          formValues.originTokenPrice = modelCopy.originTokenPrice || '';
          formValues.originCompletionTokenPrice =
            modelCopy.originCompletionTokenPrice || '';
        }

        formRef.current.setValues(formValues);
      }
    }, 0);
  };

  return (
    <>
      <Space vertical align='start' style={{ width: '100%' }}>
        <Space className='mt-2'>
          <Button
            icon={<IconPlus />}
            onClick={() => {
              resetModalState();
              setVisible(true);
            }}
          >
            {t('添加模型')}
          </Button>
          <Button type='primary' icon={<IconSave />} onClick={SubmitData}>
            {t('应用更改')}
          </Button>
          <Input
            prefix={<IconSearch />}
            placeholder={t('搜索模型名称')}
            value={searchText}
            onChange={(value) => {
              setSearchText(value);
              setCurrentPage(1);
            }}
            style={{ width: 200 }}
            showClear
          />
          <Checkbox
            checked={conflictOnly}
            onChange={(e) => {
              setConflictOnly(e.target.checked);
              setCurrentPage(1);
            }}
          >
            {t('仅显示矛盾倍率')}
          </Checkbox>
        </Space>
        <Table
          columns={columns}
          dataSource={pagedData}
          pagination={{
            currentPage: currentPage,
            pageSize: pageSize,
            total: filteredModels.length,
            onPageChange: (page) => setCurrentPage(page),
            showTotal: true,
            showSizeChanger: false,
          }}
        />
      </Space>

      <Modal
        title={isEditMode ? t('编辑模型') : t('添加模型')}
        visible={visible}
        onCancel={() => {
          resetModalState();
          setVisible(false);
        }}
        onOk={() => {
          if (currentModel) {
            // If we're in token price mode, make sure ratio values are properly set
            const valuesToSave = { ...currentModel };

            if (
              pricingMode === 'per-token' &&
              pricingSubMode === 'token-price' &&
              currentModel.tokenPrice
            ) {
              // Calculate and set ratio from token price
              const tokenPrice = parseFloat(currentModel.tokenPrice);
              valuesToSave.ratio = (tokenPrice / 2).toString();

              // Calculate and set completion ratio if both token prices are available
              if (
                currentModel.completionTokenPrice &&
                currentModel.tokenPrice
              ) {
                const completionPrice = parseFloat(
                  currentModel.completionTokenPrice,
                );
                const modelPrice = parseFloat(currentModel.tokenPrice);
                if (modelPrice > 0) {
                  valuesToSave.completionRatio = (
                    completionPrice / modelPrice
                  ).toString();
                }
              }
            }

            // 处理原始输入价格到原始模型倍率的转换
            if (
              pricingMode === 'per-token' &&
              pricingSubMode === 'token-price' &&
              currentModel.originTokenPrice
            ) {
              // Calculate and set origin ratio from origin token price
              const originTokenPrice = parseFloat(
                currentModel.originTokenPrice,
              );
              valuesToSave.originRatio = (originTokenPrice / 2).toString();
            }

            // 处理原始输出价格到原始模型补全倍率的转换
            if (
              pricingMode === 'per-token' &&
              pricingSubMode === 'token-price' &&
              currentModel.originCompletionTokenPrice &&
              currentModel.originTokenPrice
            ) {
              // Calculate and set origin completion ratio from origin completion token price
              const originCompletionTokenPrice = parseFloat(
                currentModel.originCompletionTokenPrice,
              );
              const originTokenPrice = parseFloat(
                currentModel.originTokenPrice,
              );
              if (originTokenPrice > 0) {
                valuesToSave.originCompletionRatio = (
                  originCompletionTokenPrice / originTokenPrice
                ).toString();
              }
            }

            // 处理图像Token表定价模式
            if (pricingMode === 'per-image-token') {
              // 构建ImageTokenPricing数据结构（包含固定Token表）
              if (
                currentModel.inputTextPrice &&
                currentModel.inputImagePrice &&
                currentModel.outputImagePrice
              ) {
                valuesToSave.imageTokenPricingData = {
                  input_text_price: parseFloat(currentModel.inputTextPrice),
                  input_image_price: parseFloat(currentModel.inputImagePrice),
                  output_image_price: parseFloat(currentModel.outputImagePrice),
                  token_table: {
                    low: {
                      '1024x1024': 272,
                      '1024x1536': 408,
                      '1536x1024': 400,
                    },
                    medium: {
                      '1024x1024': 1056,
                      '1024x1536': 1584,
                      '1536x1024': 1568,
                    },
                    high: {
                      '1024x1024': 4160,
                      '1024x1536': 6240,
                      '1536x1024': 6208,
                    },
                  },
                };
              }

              // 构建原始ImageTokenPricing数据结构
              if (
                currentModel.originInputTextPrice &&
                currentModel.originInputImagePrice &&
                currentModel.originOutputImagePrice
              ) {
                valuesToSave.originImageTokenPricingData = {
                  input_text_price: parseFloat(
                    currentModel.originInputTextPrice,
                  ),
                  input_image_price: parseFloat(
                    currentModel.originInputImagePrice,
                  ),
                  output_image_price: parseFloat(
                    currentModel.originOutputImagePrice,
                  ),
                  token_table: {
                    low: {
                      '1024x1024': 272,
                      '1024x1536': 408,
                      '1536x1024': 400,
                    },
                    medium: {
                      '1024x1024': 1056,
                      '1024x1536': 1584,
                      '1536x1024': 1568,
                    },
                    high: {
                      '1024x1024': 4160,
                      '1024x1536': 6240,
                      '1536x1024': 6208,
                    },
                  },
                };
              }
            }

            // 根据定价模式清空互斥字段
            if (pricingMode === 'per-image-token') {
              valuesToSave.price = '';
              valuesToSave.videoPrice = '';
              valuesToSave.ratio = '';
              valuesToSave.completionRatio = '';
            } else if (pricingMode === 'per-token') {
              valuesToSave.price = '';
              valuesToSave.videoPrice = '';
              valuesToSave.imageTokenPricingData = null;
            } else if (pricingMode === 'per-request') {
              valuesToSave.ratio = '';
              valuesToSave.completionRatio = '';
              valuesToSave.videoPrice = '';
              valuesToSave.imageTokenPricingData = null;
            } else if (pricingMode === 'per-second') {
              valuesToSave.price = '';
              valuesToSave.ratio = '';
              valuesToSave.completionRatio = '';
              valuesToSave.imageTokenPricingData = null;
            }

            addOrUpdateModel(valuesToSave);
          }
        }}
      >
        <Form getFormApi={(api) => (formRef.current = api)}>
          <Form.Input
            field='name'
            label={t('模型名称')}
            placeholder='strawberry'
            required
            disabled={isEditMode}
            onChange={(value) =>
              setCurrentModel((prev) => ({ ...prev, name: value }))
            }
          />

          <Form.Section text={t('定价模式')}>
            <div style={{ marginBottom: '16px' }}>
              <RadioGroup
                type='button'
                value={pricingMode}
                onChange={(e) => {
                  const newMode = e.target.value;
                  const oldMode = pricingMode;
                  setPricingMode(newMode);

                  // Instead of resetting all values, convert between modes
                  if (currentModel) {
                    const updatedModel = { ...currentModel };

                    // Update formRef with converted values
                    if (formRef.current) {
                      const formValues = {
                        name: updatedModel.name,
                      };

                      if (newMode === 'per-second') {
                        formValues.videoPriceInput =
                          updatedModel.videoPrice || '';
                      } else if (newMode === 'per-request') {
                        formValues.priceInput = updatedModel.price || '';
                      } else if (newMode === 'per-token') {
                        formValues.ratioInput = updatedModel.ratio || '';
                        formValues.completionRatioInput =
                          updatedModel.completionRatio || '';
                        formValues.modelTokenPrice =
                          updatedModel.tokenPrice || '';
                        formValues.completionTokenPrice =
                          updatedModel.completionTokenPrice || '';
                      }

                      formRef.current.setValues(formValues);
                    }

                    // Update the model state
                    setCurrentModel(updatedModel);
                  }
                }}
              >
                <Radio value='per-token'>{t('按量计费')}</Radio>
                <Radio value='per-request'>{t('按次计费')}</Radio>
                <Radio value='per-second'>{t('按秒计费')}</Radio>
                <Radio value='per-image-token'>{t('图像Token表计费')}</Radio>
              </RadioGroup>
            </div>
          </Form.Section>

          {pricingMode === 'per-token' && (
            <>
              <Form.Section text={t('价格设置方式')}>
                <div style={{ marginBottom: '16px' }}>
                  <RadioGroup
                    type='button'
                    value={pricingSubMode}
                    onChange={(e) => {
                      const newSubMode = e.target.value;
                      const oldSubMode = pricingSubMode;
                      setPricingSubMode(newSubMode);

                      // Handle conversion between submodes
                      if (currentModel) {
                        const updatedModel = { ...currentModel };

                        // Convert between ratio and token price with full synchronization
                        if (
                          oldSubMode === 'ratio' &&
                          newSubMode === 'token-price'
                        ) {
                          // Always calculate tokenPrice from ratio (restore original behavior)
                          if (updatedModel.ratio) {
                            updatedModel.tokenPrice =
                              calculateTokenPriceFromRatio(
                                parseFloat(updatedModel.ratio),
                              ).toString();
                          }

                          // Always calculate completionTokenPrice from completionRatio
                          if (updatedModel.completionRatio) {
                            updatedModel.completionTokenPrice = (
                              parseFloat(updatedModel.tokenPrice || 0) *
                              parseFloat(updatedModel.completionRatio)
                            ).toString();
                          }

                          // Handle original model configuration - always sync
                          if (updatedModel.originRatio) {
                            updatedModel.originTokenPrice = (
                              parseFloat(updatedModel.originRatio) * 2
                            ).toString();
                          }

                          // Handle original completion configuration - always sync
                          if (updatedModel.originCompletionRatio) {
                            updatedModel.originCompletionTokenPrice = (
                              parseFloat(updatedModel.originCompletionRatio) *
                              parseFloat(updatedModel.originTokenPrice || 0)
                            ).toString();
                          }
                        } else if (
                          oldSubMode === 'token-price' &&
                          newSubMode === 'ratio'
                        ) {
                          // Always calculate ratio from tokenPrice (restore original behavior)
                          if (updatedModel.tokenPrice) {
                            updatedModel.ratio = (
                              parseFloat(updatedModel.tokenPrice) / 2
                            ).toString();
                          }

                          // Always calculate completionRatio from completionTokenPrice
                          if (
                            updatedModel.completionTokenPrice &&
                            updatedModel.tokenPrice
                          ) {
                            updatedModel.completionRatio = (
                              parseFloat(updatedModel.completionTokenPrice) /
                              parseFloat(updatedModel.tokenPrice)
                            ).toString();
                          }

                          // Handle original model configuration - always sync
                          if (updatedModel.originTokenPrice) {
                            updatedModel.originRatio = (
                              parseFloat(updatedModel.originTokenPrice) / 2
                            ).toString();
                          }

                          // Handle original completion configuration - always sync
                          if (
                            updatedModel.originCompletionTokenPrice &&
                            updatedModel.originTokenPrice
                          ) {
                            updatedModel.originCompletionRatio = (
                              parseFloat(
                                updatedModel.originCompletionTokenPrice,
                              ) / parseFloat(updatedModel.originTokenPrice)
                            ).toString();
                          }
                        }

                        // Update the form values
                        if (formRef.current) {
                          const formValues = {};

                          if (newSubMode === 'ratio') {
                            formValues.ratioInput = updatedModel.ratio || '';
                            formValues.completionRatioInput =
                              updatedModel.completionRatio || '';
                            formValues.originRatio =
                              updatedModel.originRatio || '';
                            formValues.originCompletionRatio =
                              updatedModel.originCompletionRatio || '';
                          } else if (newSubMode === 'token-price') {
                            formValues.modelTokenPrice =
                              updatedModel.tokenPrice || '';
                            formValues.completionTokenPrice =
                              updatedModel.completionTokenPrice || '';
                            formValues.originTokenPrice =
                              updatedModel.originTokenPrice || '';
                            formValues.originCompletionTokenPrice =
                              updatedModel.originCompletionTokenPrice || '';
                          }

                          formRef.current.setValues(formValues);
                        }

                        setCurrentModel(updatedModel);
                      }
                    }}
                  >
                    <Radio value='ratio'>{t('按倍率设置')}</Radio>
                    <Radio value='token-price'>{t('按价格设置')}</Radio>
                  </RadioGroup>
                </div>
              </Form.Section>

              {pricingSubMode === 'ratio' && (
                <>
                  <Form.Input
                    field='ratioInput'
                    label={t('模型倍率')}
                    placeholder={t('输入模型倍率')}
                    onChange={(value) =>
                      setCurrentModel((prev) => ({
                        ...(prev || {}),
                        ratio: value,
                      }))
                    }
                    initValue={currentModel?.ratio || ''}
                  />
                  <Form.Input
                    field='completionRatioInput'
                    label={t('补全倍率')}
                    placeholder={t('输入补全倍率')}
                    onChange={(value) =>
                      setCurrentModel((prev) => ({
                        ...(prev || {}),
                        completionRatio: value,
                      }))
                    }
                    initValue={currentModel?.completionRatio || ''}
                  />
                </>
              )}

              {pricingSubMode === 'token-price' && (
                <>
                  <Form.Input
                    field='modelTokenPrice'
                    label={t('输入价格')}
                    onChange={(value) => {
                      handleTokenPriceChange(value);
                    }}
                    initValue={currentModel?.tokenPrice || ''}
                    suffix={t('$/1M tokens')}
                  />
                  <Form.Input
                    field='completionTokenPrice'
                    label={t('输出价格')}
                    onChange={(value) => {
                      handleCompletionTokenPriceChange(value);
                    }}
                    initValue={currentModel?.completionTokenPrice || ''}
                    suffix={t('$/1M tokens')}
                  />
                </>
              )}
            </>
          )}

          {pricingMode === 'per-request' && (
            <Form.Input
              field='priceInput'
              label={t('固定价格(每次)')}
              placeholder={t('输入每次价格')}
              onChange={(value) =>
                setCurrentModel((prev) => ({
                  ...(prev || {}),
                  price: value,
                }))
              }
              initValue={currentModel?.price || ''}
            />
          )}

          {pricingMode === 'per-second' && (
            <Form.Input
              field='videoPriceInput'
              label={t('视频每秒价格')}
              placeholder={t('输入每秒价格')}
              onChange={(value) =>
                setCurrentModel((prev) => ({
                  ...(prev || {}),
                  videoPrice: value,
                }))
              }
              initValue={currentModel?.videoPrice || ''}
              suffix={t('$/秒')}
            />
          )}

          {pricingMode === 'per-image-token' && (
            <>
              <Form.Section text={t('实际计费价格（渠道优惠后）')}>
                <Form.Input
                  field='inputTextPrice'
                  label={t('输入文本价格')}
                  placeholder={t('输入价格，例如：5.0')}
                  onChange={(value) =>
                    setCurrentModel((prev) => ({
                      ...(prev || {}),
                      inputTextPrice: value,
                    }))
                  }
                  initValue={currentModel?.inputTextPrice || ''}
                  suffix={t('$/1M tokens')}
                />
                <Form.Input
                  field='inputImagePrice'
                  label={t('输入图像价格')}
                  placeholder={t('输入价格，例如：10.0')}
                  onChange={(value) =>
                    setCurrentModel((prev) => ({
                      ...(prev || {}),
                      inputImagePrice: value,
                    }))
                  }
                  initValue={currentModel?.inputImagePrice || ''}
                  suffix={t('$/1M tokens')}
                />
                <Form.Input
                  field='outputImagePrice'
                  label={t('输出图像价格')}
                  placeholder={t('输入价格，例如：40.0')}
                  onChange={(value) =>
                    setCurrentModel((prev) => ({
                      ...(prev || {}),
                      outputImagePrice: value,
                    }))
                  }
                  initValue={currentModel?.outputImagePrice || ''}
                  suffix={t('$/1M tokens')}
                />
              </Form.Section>

              <Form.Section text={t('微软官方原价（用于对比展示）')}>
                <Form.Input
                  field='originInputTextPrice'
                  label={t('原始输入文本价格')}
                  placeholder={t('官方价格，例如：5.0')}
                  onChange={(value) =>
                    setCurrentModel((prev) => ({
                      ...(prev || {}),
                      originInputTextPrice: value,
                    }))
                  }
                  initValue={currentModel?.originInputTextPrice || ''}
                  suffix={t('$/1M tokens')}
                />
                <Form.Input
                  field='originInputImagePrice'
                  label={t('原始输入图像价格')}
                  placeholder={t('官方价格，例如：10.0')}
                  onChange={(value) =>
                    setCurrentModel((prev) => ({
                      ...(prev || {}),
                      originInputImagePrice: value,
                    }))
                  }
                  initValue={currentModel?.originInputImagePrice || ''}
                  suffix={t('$/1M tokens')}
                />
                <Form.Input
                  field='originOutputImagePrice'
                  label={t('原始输出图像价格')}
                  placeholder={t('官方价格，例如：40.0')}
                  onChange={(value) =>
                    setCurrentModel((prev) => ({
                      ...(prev || {}),
                      originOutputImagePrice: value,
                    }))
                  }
                  initValue={currentModel?.originOutputImagePrice || ''}
                  suffix={t('$/1M tokens')}
                />
              </Form.Section>
            </>
          )}

          <Form.Section text={t('原始模型配置')}>
            {/* 按次计费模式：显示原始固定价格(每次) */}
            {pricingMode === 'per-request' && (
              <Form.Input
                field='originPrice'
                label={t('原始固定价格(每次)')}
                placeholder={t('输入原始固定价格(每次)')}
                onChange={(value) =>
                  setCurrentModel((prev) => ({
                    ...(prev || {}),
                    originPrice: value,
                  }))
                }
                initValue={currentModel?.originPrice || ''}
              />
            )}

            {/* 按量计费模式：根据价格设置方式显示不同字段 */}
            {pricingMode === 'per-token' && (
              <>
                {/* 按倍率设置时：显示原始模型倍率 */}
                {pricingSubMode === 'ratio' && (
                  <Form.Input
                    field='originRatio'
                    label={t('原始模型倍率')}
                    placeholder={t('输入原始模型倍率')}
                    onChange={(value) =>
                      setCurrentModel((prev) => ({
                        ...(prev || {}),
                        originRatio: value,
                      }))
                    }
                    initValue={currentModel?.originRatio || ''}
                  />
                )}

                {/* 按价格设置时：显示原始输入价格 */}
                {pricingSubMode === 'token-price' && (
                  <Form.Input
                    field='originTokenPrice'
                    label={t('原始输入价格')}
                    placeholder={t('输入原始输入价格')}
                    suffix={t('$/1M tokens')}
                    onChange={(value) => {
                      handleOriginTokenPriceChange(value);
                    }}
                    initValue={currentModel?.originTokenPrice || ''}
                  />
                )}
              </>
            )}

            {/* 原始模型补全倍率/原始输出价格 - 根据价格设置方式显示不同字段 */}
            {pricingMode === 'per-token' && pricingSubMode === 'ratio' && (
              <Form.Input
                field='originCompletionRatio'
                label={t('原始模型补全倍率')}
                placeholder={t('输入原始模型补全倍率')}
                onChange={(value) =>
                  setCurrentModel((prev) => ({
                    ...(prev || {}),
                    originCompletionRatio: value,
                  }))
                }
                initValue={currentModel?.originCompletionRatio || ''}
              />
            )}

            {pricingMode === 'per-token' &&
              pricingSubMode === 'token-price' && (
                <Form.Input
                  field='originCompletionTokenPrice'
                  label={t('原始输出价格')}
                  placeholder={t('输入原始输出价格')}
                  suffix={t('$/1M tokens')}
                  onChange={(value) => {
                    handleOriginCompletionTokenPriceChange(value);
                  }}
                  initValue={currentModel?.originCompletionTokenPrice || ''}
                />
              )}

            {pricingMode === 'per-request' && (
              <Form.Input
                field='originCompletionRatio'
                label={t('原始模型补全倍率')}
                placeholder={t('输入原始模型补全倍率')}
                onChange={(value) =>
                  setCurrentModel((prev) => ({
                    ...(prev || {}),
                    originCompletionRatio: value,
                  }))
                }
                initValue={currentModel?.originCompletionRatio || ''}
              />
            )}
          </Form.Section>
        </Form>
      </Modal>
    </>
  );
}

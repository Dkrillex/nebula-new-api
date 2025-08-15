export const ITEMS_PER_PAGE = 10; // this value must keep same as the one defined in backend!

export const DEFAULT_ENDPOINT = '/api/ratio_config';

export const TABLE_COMPACT_MODES_KEY = 'table_compact_modes';

export const API_ENDPOINTS = [
  '/v1/chat/completions', // - 聊天对话补全，用于生成对话响应
  '/v1/responses', //  - 通用响应接口，处理各种AI模型响应
  '/v1/messages', //  - 消息处理接口，用于发送和接收消息
  '/v1beta/models', // - 模型列表查询，获取可用模型信息
  '/v1/embeddings', // - 文本嵌入生成，将文本转换为向量表示
  '/v1/rerank', // - 文档重新排序，对搜索结果进行重排优化
  '/v1/images/generations', // - 图像生成，根据文本描述生成图片
  '/v1/images/edits', // - 图像编辑，修改现有图片
  '/v1/images/variations', // - 图像变体生成，创建图片的相似版本
  '/v1/audio/speech',// - 文本转语音，将文字转换为音频
  '/v1/audio/transcriptions',// - 音频转录，将音频转换为文字
  '/v1/audio/translations'// - 音频翻译，转录并翻译音频内容
];

export const TASK_ACTION_GENERATE = 'generate';
export const TASK_ACTION_TEXT_GENERATE = 'textGenerate';
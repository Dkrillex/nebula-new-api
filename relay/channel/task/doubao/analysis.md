根据对豆包适配器代码的分析，在统一入参 `VideoRequest` 结构体中，豆包适配器使用了以下参数：

## 直接使用的参数

1. **`model`** - 模型名称，直接映射到豆包API的model字段
2. **`prompt`** - 文本提示词，作为text类型的content项
3. **`image`** - 图片URL，作为image_url类型的content项，并设置role为"first_frame"
4. **`duration`** - 视频时长，转换为`--dur`格式添加到文本提示词中
5. **`fps`** - 视频帧率，转换为`--fps`格式添加到文本提示词中
6. **`seed`** - 随机种子，转换为`--seed`格式添加到文本提示词中

## 从metadata中使用的参数

7. **`metadata["aspect_ratio"]`** - 宽高比，转换为`--rt`格式
8. **`metadata["resolution"]`** - 分辨率，转换为`--rs`格式
9. **`metadata["watermark"]`** - 水印设置，转换为`--wm`格式
10. **`metadata["camera_fixed"]`** - 固定摄像头，转换为`--cf`格式
11. **其他metadata参数** - 以`--key value`格式添加

## 未使用的参数

以下统一入参中的参数在豆包适配器中**没有被使用**：

- `width` - 视频宽度
- `height` - 视频高度
- `n` - 生成数量
- `response_format` - 响应格式
- `user` - 用户标识

这是因为豆包的火山引擎API采用了特殊的参数传递方式，将大部分参数以命令行格式（如`--dur 5`）附加到文本提示词中，而不是作为独立的JSON字段传递。
        
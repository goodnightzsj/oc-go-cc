# 离线 tokenizer 测试数据

`cl100k_base.tiktoken.gz` 是 OpenAI 的完整 cl100k_base BPE 数据，经 gzip 压缩；用于测试，生产二进制不引用此包。

来源：https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken

解压后 SHA-256：`223921b76ee99bde995b7ff738513eef100fb51d18c93597a113bcffe865b2a7`。

数据许可证见 LICENSE，来源 https://github.com/openai/tiktoken/blob/main/LICENSE 。

`Prime` 在 TestMain、m.Run 之前把固定数据放进隔离缓存，通过已安装的 tiktoken-go v0.1.8 正常加载；随后删除缓存并恢复环境变量。没有替换分词算法或按字符模拟 token 数。

import { LIQUID_FILTER_ID, LIQUID_REFRACTION } from '../design/glass'

/**
 * 液态玻璃的折射滤镜定义。
 *
 * 必须是真实存在的 DOM：CSS 的 `url(#id)` 只能引用文档里的滤镜，没法内联。
 * 所以整站挂一个 0×0 的 svg（在 App 顶层），只定义一次。
 */
export function LiquidGlassDefs() {
  const { baseFrequency, numOctaves, seed, smoothing, scale, region } = LIQUID_REFRACTION

  return (
    <svg className="liquid-defs" aria-hidden="true" focusable="false">
      <filter
        id={LIQUID_FILTER_ID}
        x={region.x}
        y={region.y}
        width={region.width}
        height={region.height}
        colorInterpolationFilters="sRGB"
      >
        {/* 低频噪声当位移图：它编码的就是"玻璃各处的厚薄" */}
        <feTurbulence
          type="fractalNoise"
          baseFrequency={baseFrequency}
          numOctaves={numOctaves}
          seed={seed}
          result="noise"
        />
        {/* 抹平。不抹平的话位移场是碎的，背景会被切成一格一格 */}
        <feGaussianBlur in="noise" stdDeviation={smoothing} result="relief" />
        {/* 噪声的红、绿通道分别当 X / Y 的位移量 */}
        <feDisplacementMap
          in="SourceGraphic"
          in2="relief"
          scale={scale}
          xChannelSelector="R"
          yChannelSelector="G"
        />
      </filter>
    </svg>
  )
}

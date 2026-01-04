import { test } from '@playwright/test'

test('Browser audio decode test', async ({ page }) => {
  test.setTimeout(90000)
  page.on('console', msg => console.log('Browser:', msg.text()))

  await page.goto('about:blank')

  const result = await page.evaluate(async () => {
    const MS = window.MediaSource
    const video = document.createElement('video')
    video.muted = true
    video.autoplay = true
    video.playsInline = true
    document.body.appendChild(video)

    const mediaSource = new MS()
    video.src = URL.createObjectURL(mediaSource)

    await new Promise<void>((r) => {
      if (mediaSource.readyState === 'open') r()
      else mediaSource.addEventListener('sourceopen', () => r(), { once: true })
    })

    const codecs = 'avc1.640029,avc1.64002A,avc1.640033,mp4a.40.2,mp4a.40.5,flac,opus'
    const ws = new WebSocket('ws://localhost:8080/go2rtc/api/ws?src=doorbell_1aa22948')
    ws.binaryType = 'arraybuffer'

    let sourceBuffer: SourceBuffer | null = null
    let codecString = ''
    const queue: Uint8Array[] = []
    let dataChunks = 0

    const processQueue = () => {
      if (!sourceBuffer || sourceBuffer.updating || queue.length === 0) return
      try {
        sourceBuffer.appendBuffer(queue.shift()!)
      } catch (e) {
        console.log('Buffer error:', e)
        if (queue.length > 5) queue.splice(0, queue.length - 3)
      }
    }

    ws.onopen = () => ws.send(JSON.stringify({ type: 'mse', value: codecs }))

    ws.onmessage = (event) => {
      if (typeof event.data === 'string') {
        const msg = JSON.parse(event.data)
        if (msg.type === 'mse') {
          codecString = msg.value
          console.log('Codec from server:', codecString)
          console.log('Supported:', MS.isTypeSupported(codecString))
          try {
            sourceBuffer = mediaSource.addSourceBuffer(codecString)
            sourceBuffer.mode = 'segments'
            sourceBuffer.addEventListener('updateend', processQueue)
            console.log('SourceBuffer created OK')
          } catch (e) {
            console.log('SourceBuffer error:', e)
          }
        }
      } else {
        dataChunks++
        queue.push(new Uint8Array(event.data))
        processQueue()
      }
    }

    // Wait for video to start
    await new Promise(r => setTimeout(r, 5000))

    // Unmute and play
    video.muted = false
    video.volume = 1.0
    try {
      await video.play()
    } catch (e) {
      console.log('Play error, trying muted:', e)
      video.muted = true
      await video.play()
      video.muted = false
    }

    console.log('Video state:', {
      muted: video.muted,
      volume: video.volume,
      paused: video.paused,
      readyState: video.readyState,
      currentTime: video.currentTime
    })

    // Check buffered ranges
    const buffered = video.buffered
    const bufferedRanges: string[] = []
    for (let i = 0; i < buffered.length; i++) {
      bufferedRanges.push(`${buffered.start(i)}-${buffered.end(i)}`)
    }

    // Wait more for data
    await new Promise(r => setTimeout(r, 2000))

    // Analyze audio with Web Audio API
    interface AudioAnalysisResult {
      samples?: number[]
      maxLevel?: number
      avgLevel?: number
      hasAudio?: boolean
      error?: string
    }
    let audioAnalysis: AudioAnalysisResult = { error: 'not attempted' }
    try {
      const audioContext = new AudioContext()
      console.log('AudioContext state:', audioContext.state)

      if (audioContext.state === 'suspended') {
        await audioContext.resume()
        console.log('AudioContext resumed:', audioContext.state)
      }

      const source = audioContext.createMediaElementSource(video)
      const analyser = audioContext.createAnalyser()
      analyser.fftSize = 256
      source.connect(analyser)
      analyser.connect(audioContext.destination) // Route to speakers

      const dataArray = new Uint8Array(analyser.frequencyBinCount)
      const samples: number[] = []

      for (let i = 0; i < 30; i++) {
        await new Promise(r => setTimeout(r, 100))
        analyser.getByteFrequencyData(dataArray)
        const avg = dataArray.reduce((a, b) => a + b, 0) / dataArray.length
        samples.push(avg)
        if (i % 10 === 0) console.log(`Sample ${i}: avg=${avg.toFixed(2)}`)
      }

      audioContext.close()

      audioAnalysis = {
        samples: samples.slice(-10),
        maxLevel: Math.max(...samples),
        avgLevel: samples.reduce((a, b) => a + b, 0) / samples.length,
        hasAudio: Math.max(...samples) > 2
      }
    } catch (e) {
      audioAnalysis = { error: (e as Error).message }
    }

    ws.close()

    // Check video's audioTracks API
    interface AudioTrack {
      enabled: boolean
      kind: string
      label: string
    }
    interface AudioTrackListWithIndex extends AudioTrackList {
      [index: number]: AudioTrack
    }
    interface AudioTracksInfo {
      length: number
      tracks: { enabled: boolean; kind: string; label: string }[]
    }
    const videoEl = video as HTMLVideoElement & { audioTracks?: AudioTrackListWithIndex }
    let audioTracksInfo: AudioTracksInfo | string = 'not available'
    if (videoEl.audioTracks) {
      audioTracksInfo = {
        length: videoEl.audioTracks.length,
        tracks: Array.from({ length: videoEl.audioTracks.length }, (_, i) => ({
          enabled: videoEl.audioTracks![i].enabled,
          kind: videoEl.audioTracks![i].kind,
          label: videoEl.audioTracks![i].label
        }))
      }
    }

    return {
      codec: codecString,
      codecSupported: MS.isTypeSupported(codecString),
      dataChunks,
      bufferedRanges,
      videoState: {
        muted: video.muted,
        volume: video.volume,
        paused: video.paused,
        readyState: video.readyState,
        currentTime: video.currentTime,
        videoWidth: video.videoWidth,
        videoHeight: video.videoHeight
      },
      audioTracks: audioTracksInfo,
      audioAnalysis
    }
  })

  console.log('\n========== AUDIO TEST RESULTS ==========')
  console.log(JSON.stringify(result, null, 2))

  if (result.audioAnalysis.hasAudio) {
    console.log('\n✅ AUDIO IS WORKING - Max level:', result.audioAnalysis.maxLevel)
  } else {
    console.log('\n❌ NO AUDIO DETECTED')
    console.log('Max level:', result.audioAnalysis.maxLevel)
    console.log('Codec:', result.codec)
    console.log('Codec supported:', result.codecSupported)
  }
})

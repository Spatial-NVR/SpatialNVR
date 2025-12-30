import { test } from '@playwright/test'

test('Test audio in actual app', async ({ page }) => {
  test.setTimeout(120000)
  page.on('console', msg => console.log('Browser:', msg.text()))

  // Navigate to the app
  await page.goto('http://localhost:8080')
  await page.waitForLoadState('networkidle')

  // Wait for video player to load
  await page.waitForTimeout(10000)

  // Find the unmute button and click it
  const unmuteButton = page.locator('button[title*="Unmute"], button[title*="Mute"]').first()

  if (await unmuteButton.count() > 0) {
    console.log('Found mute button, clicking...')
    await unmuteButton.click()
    await page.waitForTimeout(1000)
  }

  // Now analyze the audio
  const result = await page.evaluate(async () => {
    const video = document.querySelector('video') as HTMLVideoElement
    if (!video) return { error: 'No video element found' }

    console.log('Video element found')
    console.log('- src:', video.src.substring(0, 50) + '...')
    console.log('- srcObject:', video.srcObject ? 'MediaStream' : 'null')
    console.log('- muted:', video.muted)
    console.log('- volume:', video.volume)
    console.log('- paused:', video.paused)
    console.log('- readyState:', video.readyState)

    // Force unmute
    video.muted = false
    video.volume = 1.0
    console.log('After force unmute:')
    console.log('- muted:', video.muted)
    console.log('- volume:', video.volume)

    // Try to play if paused
    if (video.paused) {
      try {
        await video.play()
        console.log('Played video')
      } catch (e) {
        console.log('Play error:', e)
      }
    }

    // Wait a bit
    await new Promise(r => setTimeout(r, 2000))

    // Now analyze with AudioContext
    let audioAnalysis: any = {}
    try {
      const ctx = new AudioContext()
      console.log('AudioContext created, state:', ctx.state)

      if (ctx.state === 'suspended') {
        await ctx.resume()
        console.log('AudioContext resumed')
      }

      const source = ctx.createMediaElementSource(video)
      const analyser = ctx.createAnalyser()
      analyser.fftSize = 256
      source.connect(analyser)
      analyser.connect(ctx.destination)

      const dataArray = new Uint8Array(analyser.frequencyBinCount)
      const samples: number[] = []

      for (let i = 0; i < 30; i++) {
        await new Promise(r => setTimeout(r, 100))
        analyser.getByteFrequencyData(dataArray)
        const avg = dataArray.reduce((a, b) => a + b, 0) / dataArray.length
        samples.push(avg)
      }

      ctx.close()

      audioAnalysis = {
        samples: samples.slice(-10),
        maxLevel: Math.max(...samples),
        avgLevel: samples.reduce((a, b) => a + b, 0) / samples.length,
        hasAudio: Math.max(...samples) > 2
      }
    } catch (e) {
      audioAnalysis = { error: (e as Error).message }
    }

    return {
      videoSrc: video.src.substring(0, 80),
      hasSrcObject: !!video.srcObject,
      muted: video.muted,
      volume: video.volume,
      paused: video.paused,
      readyState: video.readyState,
      currentTime: video.currentTime,
      audioAnalysis
    }
  })

  console.log('\n========== APP AUDIO TEST RESULTS ==========')
  console.log(JSON.stringify(result, null, 2))

  if (result.audioAnalysis?.hasAudio) {
    console.log('\n✅ AUDIO IS WORKING in the app!')
  } else {
    console.log('\n❌ NO AUDIO in the app')
    console.log('Max level:', result.audioAnalysis?.maxLevel)
  }
})

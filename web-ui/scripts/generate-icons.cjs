#!/usr/bin/env node
/**
 * Simple script to generate placeholder PNG icons from the SVG
 * Run with: node scripts/generate-icons.js
 *
 * For production, consider using a tool like:
 * - sharp (npm install sharp)
 * - ImageMagick
 * - https://realfavicongenerator.net/
 */

const fs = require('fs');
const path = require('path');

const sizes = [72, 96, 128, 144, 152, 167, 180, 192, 384, 512];
const iconsDir = path.join(__dirname, '../public/icons');

// Ensure icons directory exists
if (!fs.existsSync(iconsDir)) {
  fs.mkdirSync(iconsDir, { recursive: true });
}

// Create a simple 1x1 PNG as placeholder (blue color matching theme)
// This is a minimal valid PNG file - just a placeholder until proper icons are generated
const createPlaceholderPNG = (size) => {
  // Minimal PNG: 8-byte signature + IHDR + IDAT + IEND chunks
  // This creates a solid blue square
  const signature = Buffer.from([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A]);

  // For a proper icon, you'd use sharp or canvas:
  // npm install sharp
  // const sharp = require('sharp');
  // sharp('public/icons/icon.svg').resize(size).png().toFile(`public/icons/icon-${size}x${size}.png`)

  console.log(`Icon ${size}x${size}: Use 'npm run generate-icons' with sharp installed, or generate at realfavicongenerator.net`);
};

console.log('PWA Icon Generation');
console.log('==================');
console.log('');
console.log('The SVG icon is at: public/icons/icon.svg');
console.log('');
console.log('To generate PNG icons, you have these options:');
console.log('');
console.log('1. Use realfavicongenerator.net (recommended):');
console.log('   - Upload the SVG at https://realfavicongenerator.net/');
console.log('   - Download the icon package');
console.log('   - Extract to public/icons/');
console.log('');
console.log('2. Use ImageMagick (if installed):');
sizes.forEach(size => {
  console.log(`   convert -background none -resize ${size}x${size} public/icons/icon.svg public/icons/icon-${size}x${size}.png`);
});
console.log('');
console.log('3. Install sharp and run this script:');
console.log('   npm install sharp --save-dev');
console.log('   # Then uncomment the sharp code in this script');
console.log('');

// Check if sharp is available
try {
  const sharp = require('sharp');
  console.log('Sharp is installed! Generating icons...');

  const svgPath = path.join(iconsDir, 'icon.svg');
  if (!fs.existsSync(svgPath)) {
    console.error('SVG icon not found at:', svgPath);
    process.exit(1);
  }

  Promise.all(sizes.map(size =>
    sharp(svgPath)
      .resize(size, size)
      .png()
      .toFile(path.join(iconsDir, `icon-${size}x${size}.png`))
      .then(() => console.log(`Generated: icon-${size}x${size}.png`))
  )).then(() => {
    console.log('All icons generated!');
  }).catch(err => {
    console.error('Error generating icons:', err);
  });
} catch (e) {
  // Sharp not installed, just show instructions
}

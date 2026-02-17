function calculateTotalPriceWithDiscountApplied(originalPrice, discountPercentage) {
  const discountAmount = originalPrice * (discountPercentage / 100);
  const finalPrice = originalPrice - discountAmount;
  return finalPrice;
}

const result = calculateTotalPriceWithDiscountApplied(100, 20);
if (result !== 80) {
  process.exit(1);
}
console.log("minify test passed: " + result);

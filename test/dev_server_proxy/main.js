fetch("/api/health")
  .then(r => r.json())
  .then(data => {
    document.getElementById("result").textContent = JSON.stringify(data, null, 2);
  })
  .catch(err => {
    document.getElementById("result").textContent = "Error: " + err.message;
  });

resource "google_compute_ssl_certificate" "rune_production_explorer" {
  name = "rune-production-explorer-ssl-certificate"
  private_key = file("./ssl-certificates/rune-production-explorer.key")
  certificate = file("./ssl-certificates/rune-production-explorer.crt")

  lifecycle {
    create_before_destroy = true
  }
}

resource "google_compute_ssl_certificate" "rune_production_monitoring_hub" {
  name = "rune-production-monitoring-hub-ssl-certificate"
  private_key = file("./ssl-certificates/rune-production-monitoring-hub.key")
  certificate = file("./ssl-certificates/rune-production-monitoring-hub.crt")

  lifecycle {
    create_before_destroy = true
  }
}

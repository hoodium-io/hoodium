resource "google_compute_ssl_certificate" "rune_staging_explorer" {
  name = "rune-staging-explorer-ssl-certificate"
  private_key = file("./ssl-certificates/rune-staging-explorer.key")
  certificate = file("./ssl-certificates/rune-staging-explorer.crt")

  lifecycle {
    create_before_destroy = true
  }
}

resource "google_compute_ssl_certificate" "rune_staging_rpc" {
  name = "rune-staging-rpc-ssl-certificate"
  private_key = file("./ssl-certificates/rune-staging-rpc.key")
  certificate = file("./ssl-certificates/rune-staging-rpc.crt")

  lifecycle {
    create_before_destroy = true
  }
}

resource "google_compute_ssl_certificate" "rune_staging_rpc-ws" {
  name = "rune-staging-rpc-ws-ssl-certificate"
  private_key = file("./ssl-certificates/rune-staging-rpc-ws.key")
  certificate = file("./ssl-certificates/rune-staging-rpc-ws.crt")

  lifecycle {
    create_before_destroy = true
  }
}

resource "google_compute_ssl_certificate" "rune_staging_safe" {
  name = "rune-staging-safe-ssl-certificate"
  private_key = file("./ssl-certificates/rune-staging-safe.key")
  certificate = file("./ssl-certificates/rune-staging-safe.crt")

  lifecycle {
    create_before_destroy = true
  }
}

resource "google_compute_ssl_certificate" "rune_staging_monitoring_hub" {
  name = "rune-staging-monitoring-hub-ssl-certificate"
  private_key = file("./ssl-certificates/rune-staging-monitoring-hub.key")
  certificate = file("./ssl-certificates/rune-staging-monitoring-hub.crt")

  lifecycle {
    create_before_destroy = true
  }
}
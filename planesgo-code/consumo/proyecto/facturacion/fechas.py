from datetime import date, timedelta


def vencimiento(emision, dias=30):
    return emision + timedelta(days=dias)


def trimestre(fecha: date):
    return (fecha.month - 1) // 3 + 1
